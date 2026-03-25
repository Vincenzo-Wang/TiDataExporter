package s3

import (
	"context"
	"io"
	"time"
)

// StorageClient 统一存储客户端接口
type StorageClient interface {
	// Upload 上传文件
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error

	// Delete 删除文件
	Delete(ctx context.Context, key string) error

	// DeleteByPrefix 批量删除指定前缀的文件
	DeleteByPrefix(ctx context.Context, prefix string) error

	// GetPresignedURL 获取预签名URL
	GetPresignedURL(ctx context.Context, key string, expire time.Duration) (string, error)

	// Head 检查文件是否存在并获取元数据
	Head(ctx context.Context, key string) (HeadResult, error)

	// Exists 检查文件是否存在
	Exists(ctx context.Context, key string) (bool, error)

	// Size 获取文件大小
	Size(ctx context.Context, key string) (int64, error)

	// GetFileURL 获取文件的公开URL（不使用预签名）
	GetFileURL(key string) string
}

// HeadResult 文件元数据结果
type HeadResult struct {
	ContentLength int64
	ContentType   string
	LastModified  time.Time
	ETag          string
}

// Config 存储配置
type Config struct {
	Provider   string
	Endpoint   string
	AccessKey  string
	SecretKey  string
	Bucket     string
	Region     string
	PathPrefix string
}

// ProviderType 存储厂商类型
const (
	ProviderAWS   = "aws"
	ProviderAliyun = "aliyun"
)
