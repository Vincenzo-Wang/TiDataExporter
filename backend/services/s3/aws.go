package s3

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// AWSClient AWS S3客户端
type AWSClient struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewAWSClient 创建AWS S3客户端
func NewAWSClient(ctx context.Context, cfg Config) (*AWSClient, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if cfg.Endpoint != "" {
			return aws.Endpoint{
				URL:           cfg.Endpoint,
				SigningRegion: cfg.Region,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey, cfg.SecretKey, "",
		)),
		config.WithRegion(cfg.Region),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	return &AWSClient{
		client: s3.NewFromConfig(awsCfg),
		bucket: cfg.Bucket,
		prefix: cfg.PathPrefix,
	}, nil
}

// Upload 上传文件
func (c *AWSClient) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	uploader := manager.NewUploader(c.client)

	input := &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(c.getFullKey(key)),
		Body:          reader,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	}

	_, err := uploader.Upload(ctx, input)
	return err
}

// Delete 删除文件
func (c *AWSClient) Delete(ctx context.Context, key string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.getFullKey(key)),
	})
	return err
}

// DeleteByPrefix 批量删除指定前缀的文件
func (c *AWSClient) DeleteByPrefix(ctx context.Context, prefix string) error {
	paginator := s3.NewListObjectsV2Paginator(c.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(c.getFullKey(prefix)),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		if len(page.Contents) == 0 {
			break
		}

		var objects []types.ObjectIdentifier
		for _, obj := range page.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: obj.Key})
		}

		_, err = c.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(c.bucket),
			Delete: &types.Delete{
				Objects: objects,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// GetPresignedURL 获取预签名URL
func (c *AWSClient) GetPresignedURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(c.client)

	req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.getFullKey(key)),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expire
	})
	if err != nil {
		return "", err
	}

	return req.URL, nil
}

// Head 检查文件是否存在并获取元数据
func (c *AWSClient) Head(ctx context.Context, key string) (HeadResult, error) {
	resp, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.getFullKey(key)),
	})
	if err != nil {
		return HeadResult{}, err
	}

	result := HeadResult{}
	if resp.ContentLength != nil {
		result.ContentLength = *resp.ContentLength
	}
	if resp.ContentType != nil {
		result.ContentType = *resp.ContentType
	}
	if resp.LastModified != nil {
		result.LastModified = *resp.LastModified
	}
	if resp.ETag != nil {
		result.ETag = *resp.ETag
	}

	return result, nil
}

// Exists 检查文件是否存在
func (c *AWSClient) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.Head(ctx, key)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// Size 获取文件大小
func (c *AWSClient) Size(ctx context.Context, key string) (int64, error) {
	resp, err := c.Head(ctx, key)
	return resp.ContentLength, err
}

// GetFileURL 获取文件的公开URL（不使用预签名）
func (c *AWSClient) GetFileURL(key string) string {
	endpoint := c.client.Options().BaseEndpoint
	if endpoint == nil {
		return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", c.bucket, c.getFullKey(key))
	}
	return fmt.Sprintf("https://%s/%s/%s", *endpoint, c.bucket, c.getFullKey(key))
}

func (c *AWSClient) getFullKey(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + "/" + key
}

// ============================================================
// 以下为向后兼容的旧 API，保留以支持现有代码
// ============================================================

// Client 旧版客户端（向后兼容）
type Client = AWSClient

// NewClient 创建S3客户端（向后兼容）
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	return NewAWSClient(ctx, cfg)
}

// HeadObjectOutput 旧版返回类型（向后兼容）
type HeadObjectOutput = s3.HeadObjectOutput

// HeadLegacy 旧版 Head 方法（返回 s3.HeadObjectOutput）
func (c *AWSClient) HeadLegacy(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	return c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.getFullKey(key)),
	})
}

// UploadFromPath 从本地路径上传文件
func (c *AWSClient) UploadFromPath(ctx context.Context, localPath, remoteKey string) (int64, error) {
	_ = localPath
	_ = remoteKey
	return 0, fmt.Errorf("not implemented: use Upload method with io.Reader")
}
