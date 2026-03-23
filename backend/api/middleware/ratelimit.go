package middleware

import (
	"context"
	"time"

	"claw-export-platform/api/utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	RequestsPerMinute int           // 每分钟请求数
	BurstSize         int           // 突发容量
	KeyPrefix         string        // Redis 键前缀
	BlockDuration     time.Duration // 超限后阻塞时间
}

// DefaultRateLimitConfig 默认限流配置
var DefaultRateLimitConfig = RateLimitConfig{
	RequestsPerMinute: 100,
	BurstSize:         20,
	KeyPrefix:         "ratelimit",
	BlockDuration:     time.Minute,
}

// RateLimiter 限流器接口
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// TokenBucketLimiter 令牌桶限流器
type TokenBucketLimiter struct {
	rdb        *redis.Client
	config     RateLimitConfig
	logger     *zap.Logger
}

// NewTokenBucketLimiter 创建令牌桶限流器
func NewTokenBucketLimiter(rdb *redis.Client, config RateLimitConfig, logger *zap.Logger) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		rdb:    rdb,
		config: config,
		logger: logger,
	}
}

// Allow 检查是否允许请求
func (l *TokenBucketLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().UnixNano()
	rate := float64(l.config.RequestsPerMinute) / 60.0 // 每秒速率
	capacity := l.config.BurstSize

	// 使用 Redis Lua 脚本实现令牌桶算法
	script := redis.NewScript(`
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local rate = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		local requested = tonumber(ARGV[4])

		-- 获取当前桶状态
		local last_refill = redis.call('HGET', key, 'last_refill')
		local tokens = redis.call('HGET', key, 'tokens')

		if last_refill == false then
			-- 初始化桶
			redis.call('HMSET', key, 'tokens', capacity, 'last_refill', now)
			redis.call('PEXPIRE', key, 60000)
			tokens = capacity
			last_refill = now
		else
			tokens = tonumber(tokens)
			last_refill = tonumber(last_refill)

			-- 计算新增的令牌
			local elapsed = (now - last_refill) / 1000000000
			local new_tokens = elapsed * rate
			tokens = math.min(capacity, tokens + new_tokens)

			-- 更新最后填充时间
			redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
		end

		-- 检查是否有足够的令牌
		if tokens >= requested then
			redis.call('HSET', key, 'tokens', tokens - requested)
			return {1, tokens - requested}
		else
			return {0, tokens}
		end
	`)

	result, err := script.Run(ctx, l.rdb, []string{key}, capacity, rate, now, 1).Result()
	if err != nil {
		l.logger.Error("rate limit check failed", zap.Error(err))
		return true, nil // 出错时允许请求通过
	}

	values := result.([]interface{})
	allowed := values[0].(int64) == 1
	return allowed, nil
}

// RateLimit 限流中间件（基于IP）
func RateLimit(limiter RateLimiter, config RateLimitConfig, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := config.KeyPrefix + ":ip:" + ip

		allowed, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			logger.Error("rate limit check failed", zap.Error(err))
			// 出错时允许请求通过
			c.Next()
			return
		}

		if !allowed {
			utils.RateLimited(c, "请求频率超限，请稍后重试")
			c.Header("Retry-After", "60")
			c.Abort()
			return
		}

		c.Next()
	}
}

// TenantRateLimit 租户级限流中间件
func TenantRateLimit(limiter RateLimiter, config RateLimitConfig, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID, exists := c.Get(string(ContextKeyTenantID))
		if !exists {
			c.Next()
			return
		}

		key := config.KeyPrefix + ":tenant:" + string(rune(tenantID.(int64)))

		allowed, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			logger.Error("tenant rate limit check failed", zap.Error(err))
			c.Next()
			return
		}

		if !allowed {
			utils.RateLimited(c, "请求频率超限，请稍后重试")
			c.Header("Retry-After", "60")
			c.Abort()
			return
		}

		c.Next()
	}
}

// SlidingWindowLimiter 滑动窗口限流器
type SlidingWindowLimiter struct {
	rdb    *redis.Client
	config RateLimitConfig
	logger *zap.Logger
}

// NewSlidingWindowLimiter 创建滑动窗口限流器
func NewSlidingWindowLimiter(rdb *redis.Client, config RateLimitConfig, logger *zap.Logger) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		rdb:    rdb,
		config: config,
		logger: logger,
	}
}

// Allow 检查是否允许请求（滑动窗口算法）
func (l *SlidingWindowLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().UnixMilli()
	windowStart := now - 60000 // 1分钟窗口

	script := redis.NewScript(`
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])

		-- 移除过期的请求记录
		redis.call('ZREMRANGEBYSCORE', key, 0, window_start)

		-- 获取当前窗口内的请求数
		local count = redis.call('ZCARD', key)

		if count < limit then
			-- 添加当前请求
			redis.call('ZADD', key, now, now .. ':' .. math.random())
			redis.call('PEXPIRE', key, 60000)
			return {1, count + 1}
		else
			return {0, count}
		end
	`)

	result, err := script.Run(ctx, l.rdb, []string{key}, now, windowStart, l.config.RequestsPerMinute).Result()
	if err != nil {
		l.logger.Error("sliding window rate limit check failed", zap.Error(err))
		return true, nil
	}

	values := result.([]interface{})
	allowed := values[0].(int64) == 1
	return allowed, nil
}
