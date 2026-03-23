package middleware

import (
	"net/http"
	"strings"
	"time"

	"claw-export-platform/api/utils"
	"claw-export-platform/config"
	"claw-export-platform/models"
	"claw-export-platform/pkg/encryption"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ContextKey 上下文键类型
type ContextKey string

const (
	ContextKeyTenantID   ContextKey = "tenant_id"
	ContextKeyAdminID    ContextKey = "admin_id"
	ContextKeyAdmin      ContextKey = "admin"
	ContextKeyTenant     ContextKey = "tenant"
	ContextKeyRequestID  ContextKey = "request_id"
)

// APIKeyAuth API Key认证中间件（用于开放API）
func APIKeyAuth(db *gorm.DB, encryptor *encryption.Encryptor, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		apiSecret := c.GetHeader("X-API-Secret")

		if apiKey == "" || apiSecret == "" {
			utils.Unauthorized(c, "缺少API Key或Secret")
			c.Abort()
			return
		}

		// 查询租户
		var tenant models.Tenant
		if err := db.Where("api_key = ?", apiKey).First(&tenant).Error; err != nil {
			logger.Warn("invalid api key", zap.String("api_key", apiKey), zap.Error(err))
			utils.Unauthorized(c, "API Key或Secret错误")
			c.Abort()
			return
		}

		// 验证租户状态
		if tenant.Status != 1 {
			utils.TenantDisabled(c)
			c.Abort()
			return
		}

		// 解密存储的 API Secret 并验证
		decryptedSecret, err := encryptor.Decrypt(tenant.APISecretEncrypted)
		if err != nil {
			logger.Error("failed to decrypt api secret", zap.Int64("tenant_id", tenant.ID), zap.Error(err))
			utils.InternalError(c, "认证服务异常")
			c.Abort()
			return
		}

		// 使用常量时间比较防止时序攻击
		if !constantTimeEqual(apiSecret, decryptedSecret) {
			logger.Warn("invalid api secret", zap.String("api_key", apiKey))
			utils.Unauthorized(c, "API Key或Secret错误")
			c.Abort()
			return
		}

		// 设置上下文
		c.Set(string(ContextKeyTenantID), tenant.ID)
		c.Set(string(ContextKeyTenant), &tenant)

		c.Next()
	}
}

// constantTimeEqual 常量时间比较，防止时序攻击
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

// JWTClaims JWT声明
type JWTClaims struct {
	AdminID  int64  `json:"admin_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// JWTAuth JWT认证中间件（用于管理API）
func JWTAuth(cfg *config.Config, db *gorm.DB, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.Unauthorized(c, "缺少Authorization头")
			c.Abort()
			return
		}

		// 解析Bearer Token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			utils.Unauthorized(c, "Authorization格式错误")
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 解析Token
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.Security.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			logger.Warn("invalid jwt token", zap.Error(err))
			utils.Unauthorized(c, "Token无效或已过期")
			c.Abort()
			return
		}

		// 查询管理员
		var admin models.Admin
		if err := db.First(&admin, claims.AdminID).Error; err != nil {
			utils.Unauthorized(c, "用户不存在")
			c.Abort()
			return
		}

		// 验证管理员状态
		if admin.Status != 1 {
			utils.Forbidden(c, "账号已被禁用")
			c.Abort()
			return
		}

		// 设置上下文
		c.Set(string(ContextKeyAdminID), admin.ID)
		c.Set(string(ContextKeyAdmin), &admin)

		c.Next()
	}
}

// GenerateToken 生成JWT Token
func GenerateToken(cfg *config.Config, admin *models.Admin) (string, error) {
	claims := &JWTClaims{
		AdminID:  admin.ID,
		Username: admin.Username,
		Role:     admin.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(cfg.Security.TokenExpireHour) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "claw-export-platform",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.Security.JWTSecret))
}

// RequestID 请求ID中间件
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set(string(ContextKeyRequestID), requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// Logger 日志中间件
func Logger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		logger.Info("request",
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.Duration("latency", latency),
			zap.String("user-agent", c.Request.UserAgent()),
		)
	}
}

// Recovery 恢复中间件
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
				)
				utils.InternalError(c, "服务器内部错误")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-API-Key, X-API-Secret")
		c.Header("Access-Control-Expose-Headers", "Content-Length, X-Request-ID")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
