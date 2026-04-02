package redis

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Config Redis 配置
type Config struct {
	Addr               string
	Username           string
	Password           string
	DB                 int
	TLSEnabled         bool
	InsecureSkipVerify bool
	ServerName         string
	CAFile             string
	CertFile           string
	KeyFile            string
	DialTimeout        time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	PoolSize           int
	MinIdleConns       int
	MaxRetries         int
	StreamName         string
	ConsumerGroup      string
	ConsumerName       string
	PendingTimeout     time.Duration
	BlockTimeout       time.Duration
}

// Client Redis 客户端包装
type Client struct {
	*goredis.Client
	cfg Config
}

// NewClient 创建 Redis 客户端
func NewClient(cfg Config) (*Client, error) {
	cfg = withDefaults(cfg)

	options := &goredis.Options{
		Addr:         cfg.Addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxRetries:   cfg.MaxRetries,
	}

	if cfg.TLSEnabled {
		tlsConfig, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
		options.TLSConfig = tlsConfig
	}

	client := goredis.NewClient(options)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Client{Client: client, cfg: cfg}, nil
}

func withDefaults(cfg Config) Config {
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = 10 * time.Second
	}
	if cfg.PoolSize <= 0 {
		cfg.PoolSize = 20
	}
	if cfg.MinIdleConns <= 0 {
		cfg.MinIdleConns = 5
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}
	if strings.TrimSpace(cfg.StreamName) == "" {
		cfg.StreamName = "export:tasks"
	}
	if strings.TrimSpace(cfg.ConsumerGroup) == "" {
		cfg.ConsumerGroup = "export-workers"
	}
	if cfg.PendingTimeout <= 0 {
		cfg.PendingTimeout = 30 * time.Minute
	}
	if cfg.BlockTimeout <= 0 {
		cfg.BlockTimeout = 5 * time.Second
	}
	return cfg
}

func buildTLSConfig(cfg Config) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		ServerName:         strings.TrimSpace(cfg.ServerName),
	}

	if strings.TrimSpace(cfg.CAFile) != "" {
		caPEM, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read REDIS TLS CA file: %w", err)
		}
		rootCAs, err := x509.SystemCertPool()
		if err != nil || rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}
		if !rootCAs.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("failed to parse REDIS TLS CA file")
		}
		tlsConfig.RootCAs = rootCAs
	}

	if strings.TrimSpace(cfg.CertFile) != "" || strings.TrimSpace(cfg.KeyFile) != "" {
		if strings.TrimSpace(cfg.CertFile) == "" || strings.TrimSpace(cfg.KeyFile) == "" {
			return nil, fmt.Errorf("REDIS_TLS_CERT_FILE and REDIS_TLS_KEY_FILE must be configured together")
		}
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load REDIS TLS client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// EnsureStreamAndGroup 确保 Stream 和 Consumer Group 存在
func (c *Client) EnsureStreamAndGroup(ctx context.Context) error {
	err := c.XGroupCreateMkStream(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}
	return nil
}

// StreamMessage 流消息
type StreamMessage struct {
	ID     string
	Fields map[string]interface{}
}

// AddMessage 向 Stream 添加消息
func (c *Client) AddMessage(ctx context.Context, fields map[string]interface{}) (string, error) {
	return c.XAdd(ctx, &goredis.XAddArgs{
		Stream: c.cfg.StreamName,
		Values: fields,
	}).Result()
}

// ReadMessages 从 Stream 读取消息
func (c *Client) ReadMessages(ctx context.Context, count int64) ([]StreamMessage, error) {
	consumerName := strings.TrimSpace(c.cfg.ConsumerName)
	if consumerName == "" {
		return nil, fmt.Errorf("redis consumer name is required")
	}

	streams, err := c.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    c.cfg.ConsumerGroup,
		Consumer: consumerName,
		Streams:  []string{c.cfg.StreamName, ">"},
		Count:    count,
		Block:    c.cfg.BlockTimeout,
	}).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil
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
	return c.XAck(ctx, c.cfg.StreamName, c.cfg.ConsumerGroup, msgID).Err()
}

// GetPendingMessages 获取待处理消息
func (c *Client) GetPendingMessages(ctx context.Context) ([]goredis.XPendingExt, error) {
	return c.XPendingExt(ctx, &goredis.XPendingExtArgs{
		Stream: c.cfg.StreamName,
		Group:  c.cfg.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  100,
	}).Result()
}

// ClaimMessage 认领超时消息
func (c *Client) ClaimMessage(ctx context.Context, msgID string) (*goredis.XMessage, error) {
	consumerName := strings.TrimSpace(c.cfg.ConsumerName)
	if consumerName == "" {
		return nil, fmt.Errorf("redis consumer name is required")
	}

	messages, err := c.XClaim(ctx, &goredis.XClaimArgs{
		Stream:   c.cfg.StreamName,
		Group:    c.cfg.ConsumerGroup,
		Consumer: consumerName,
		MinIdle:  c.cfg.PendingTimeout,
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
	return c.XDel(ctx, c.cfg.StreamName, msgID).Err()
}

// PendingTimeout 返回超时认领阈值
func (c *Client) PendingTimeout() time.Duration {
	return c.cfg.PendingTimeout
}

// ConsumerName 返回消费者名称
func (c *Client) ConsumerName() string {
	return c.cfg.ConsumerName
}

// Close 关闭连接
func (c *Client) Close() error {
	return c.Client.Close()
}
