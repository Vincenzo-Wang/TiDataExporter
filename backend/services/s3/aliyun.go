package s3

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// AliyunClient 阿里云OSS客户端
type AliyunClient struct {
	client *oss.Client
	bucket *oss.Bucket
	prefix string
}

// NewAliyunClient 创建阿里云OSS客户端
func NewAliyunClient(ctx context.Context, cfg Config) (*AliyunClient, error) {
	// 阿里云 OSS endpoint 格式: oss-{region}.aliyuncs.com 或自定义域名
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("oss-%s.aliyuncs.com", cfg.Region)
	}

	client, err := oss.New(endpoint, cfg.AccessKey, cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create aliyun oss client: %w", err)
	}

	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket: %w", err)
	}

	return &AliyunClient{
		client: client,
		bucket: bucket,
		prefix: cfg.PathPrefix,
	}, nil
}

// Upload 上传文件
func (c *AliyunClient) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	options := []oss.Option{
		oss.ContentType(contentType),
		oss.ContentLength(size),
	}

	err := c.bucket.PutObject(c.getFullKey(key), reader, options...)
	return err
}

// Delete 删除文件
func (c *AliyunClient) Delete(ctx context.Context, key string) error {
	return c.bucket.DeleteObject(c.getFullKey(key))
}

// DeleteByPrefix 批量删除指定前缀的文件
func (c *AliyunClient) DeleteByPrefix(ctx context.Context, prefix string) error {
	fullPrefix := c.getFullKey(prefix)

	// 列出所有匹配前缀的对象
	marker := ""
	for {
		lsRes, err := c.bucket.ListObjects(oss.Prefix(fullPrefix), oss.Marker(marker), oss.MaxKeys(1000))
		if err != nil {
			return err
		}

		if len(lsRes.Objects) == 0 {
			break
		}

		// 构建删除列表
		var objects []string
		for _, obj := range lsRes.Objects {
			objects = append(objects, obj.Key)
		}

		// 批量删除
		if len(objects) > 0 {
			_, err = c.bucket.DeleteObjects(objects)
			if err != nil {
				return err
			}
		}

		if !lsRes.IsTruncated {
			break
		}
		marker = lsRes.NextMarker
	}

	return nil
}

// GetPresignedURL 获取预签名URL
func (c *AliyunClient) GetPresignedURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	signedURL, err := c.bucket.SignURL(c.getFullKey(key), oss.HTTPGet, int64(expire.Seconds()))
	if err != nil {
		return "", err
	}
	return signedURL, nil
}

// Head 检查文件是否存在并获取元数据
func (c *AliyunClient) Head(ctx context.Context, key string) (HeadResult, error) {
	meta, err := c.bucket.GetObjectDetailedMeta(c.getFullKey(key))
	if err != nil {
		return HeadResult{}, err
	}

	result := HeadResult{}

	// Content-Length
	if contentLength := meta.Get("Content-Length"); contentLength != "" {
		var length int64
		fmt.Sscanf(contentLength, "%d", &length)
		result.ContentLength = length
	}

	// Content-Type
	if contentType := meta.Get("Content-Type"); contentType != "" {
		result.ContentType = contentType
	}

	// Last-Modified
	if lastModified := meta.Get("Last-Modified"); lastModified != "" {
		if t, err := http.ParseTime(lastModified); err == nil {
			result.LastModified = t
		}
	}

	// ETag
	if etag := meta.Get("ETag"); etag != "" {
		result.ETag = etag
	}

	return result, nil
}

// Exists 检查文件是否存在
func (c *AliyunClient) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.Head(ctx, key)
	if err != nil {
		// 检查是否是对象不存在的错误
		if ossErr, ok := err.(oss.ServiceError); ok {
			if ossErr.StatusCode == 404 || ossErr.Code == "NoSuchKey" {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

// Size 获取文件大小
func (c *AliyunClient) Size(ctx context.Context, key string) (int64, error) {
	resp, err := c.Head(ctx, key)
	return resp.ContentLength, err
}

// GetFileURL 获取文件的公开URL（不使用预签名）
func (c *AliyunClient) GetFileURL(key string) string {
	// 阿里云 OSS 标准URL格式: https://{bucket}.{endpoint}/{key}
	endpoint := c.client.Config.Endpoint
	return fmt.Sprintf("https://%s.%s/%s", c.bucket.BucketName, endpoint, c.getFullKey(key))
}

func (c *AliyunClient) getFullKey(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + "/" + key
}

// GetBucket 获取 OSS Bucket 对象（用于特殊操作）
func (c *AliyunClient) GetBucket() *oss.Bucket {
	return c.bucket
}

// GetClient 获取 OSS Client 对象（用于特殊操作）
func (c *AliyunClient) GetClient() *oss.Client {
	return c.client
}

// GenerateSignedPutURL 生成带签名的上传URL（用于客户端直传）
func (c *AliyunClient) GenerateSignedPutURL(ctx context.Context, key string, expire time.Duration, contentType string) (string, error) {
	key = c.getFullKey(key)
	options := []oss.Option{}
	if contentType != "" {
		options = append(options, oss.ContentType(contentType))
	}

	signedURL, err := c.bucket.SignURL(key, oss.HTTPPut, int64(expire.Seconds()), options...)
	if err != nil {
		return "", err
	}

	// 解析URL并返回
	return signedURL, nil
}

// CopyObject 复制对象
func (c *AliyunClient) CopyObject(ctx context.Context, srcKey, destKey string) error {
	_, err := c.bucket.CopyObject(c.getFullKey(srcKey), c.getFullKey(destKey))
	return err
}

// GetObject 获取对象内容
func (c *AliyunClient) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	return c.bucket.GetObject(c.getFullKey(key))
}

// ListObjects 列出指定前缀的对象
func (c *AliyunClient) ListObjects(ctx context.Context, prefix string, maxKeys int) ([]oss.ObjectProperties, error) {
	fullPrefix := c.getFullKey(prefix)
	lsRes, err := c.bucket.ListObjects(oss.Prefix(fullPrefix), oss.MaxKeys(maxKeys))
	if err != nil {
		return nil, err
	}
	return lsRes.Objects, nil
}
