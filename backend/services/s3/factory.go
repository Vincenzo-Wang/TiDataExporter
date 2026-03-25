package s3

import (
	"context"
	"fmt"
)

// NewStorageClient 根据配置创建对应的存储客户端
func NewStorageClient(ctx context.Context, cfg Config) (StorageClient, error) {
	switch cfg.Provider {
	case ProviderAWS, "":
		return NewAWSClient(ctx, cfg)
	case ProviderAliyun:
		return NewAliyunClient(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported storage provider: %s", cfg.Provider)
	}
}
