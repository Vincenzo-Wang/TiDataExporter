package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config Redis配置
type Config struct {
	Addr     string
	Password string
	DB       int
}

// Client Redis客户端包装
type Client struct {
	*redis.Client
}

// NewClient 创建Redis客户端
func NewClient(cfg Config) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		PoolSize:     20,
		MinIdleConns: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{client}, nil
}

// StreamName 任务队列流名称
const (
	StreamName       = "export:tasks"
	ConsumerGroup    = "export-workers"
	ConsumerName     = "worker-1"
	PendingTimeout   = 30 * time.Minute
	ClaimInterval    = 5 * time.Minute
	BlockTimeout     = 5 * time.Second
)

// EnsureStreamAndGroup 确保Stream和Consumer Group存在
func (c *Client) EnsureStreamAndGroup(ctx context.Context) error {
	// 创建Stream（如果不存在）
	err := c.XGroupCreateMkStream(ctx, StreamName, ConsumerGroup, "0").Err()
	if err != nil {
		// 如果group已存在，忽略错误
		if err.Error() != "BUSYGROUP Consumer Group name already exists" {
			return fmt.Errorf("failed to create consumer group: %w", err)
		}
	}
	return nil
}

// StreamMessage 流消息
type StreamMessage struct {
	ID     string
	Fields map[string]interface{}
}

// AddMessage 向Stream添加消息
func (c *Client) AddMessage(ctx context.Context, fields map[string]interface{}) (string, error) {
	return c.XAdd(ctx, &redis.XAddArgs{
		Stream: StreamName,
		Values: fields,
	}).Result()
}

// ReadMessages 从Stream读取消息
func (c *Client) ReadMessages(ctx context.Context, count int64) ([]StreamMessage, error) {
	streams, err := c.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    ConsumerGroup,
		Consumer: ConsumerName,
		Streams:  []string{StreamName, ">"},
		Count:    count,
		Block:    BlockTimeout,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 无消息
		}
		return nil, err
	}

	var messages []StreamMessage
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			messages = append(messages, StreamMessage{
				ID:     msg.ID,
				Fields: msg.Values,
			})
		}
	}
	return messages, nil
}

// AckMessage 确认消息已处理
func (c *Client) AckMessage(ctx context.Context, msgID string) error {
	return c.XAck(ctx, StreamName, ConsumerGroup, msgID).Err()
}

// GetPendingMessages 获取待处理的消息（用于超时恢复）
func (c *Client) GetPendingMessages(ctx context.Context) ([]redis.XPendingExt, error) {
	return c.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream:   StreamName,
		Group:    ConsumerGroup,
		Start:    "-",
		End:      "+",
		Count:    100,
	}).Result()
}

// ClaimMessage 认领超时消息
func (c *Client) ClaimMessage(ctx context.Context, msgID string) (*redis.XMessage, error) {
	messages, err := c.XClaim(ctx, &redis.XClaimArgs{
		Stream:   StreamName,
		Group:    ConsumerGroup,
		Consumer: ConsumerName,
		MinIdle:  PendingTimeout,
		Messages: []string{msgID},
	}).Result()
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}
	return &messages[0], nil
}

// DeleteMessage 删除消息
func (c *Client) DeleteMessage(ctx context.Context, msgID string) error {
	return c.XDel(ctx, StreamName, msgID).Err()
}

// Close 关闭连接
func (c *Client) Close() error {
	return c.Client.Close()
}
