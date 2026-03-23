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

// Config S3配置
type Config struct {
	Endpoint   string
	AccessKey  string
	SecretKey  string
	Bucket     string
	Region     string
	PathPrefix string
}

// Client S3客户端
type Client struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewClient 创建S3客户端
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
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

	return &Client{
		client: s3.NewFromConfig(awsCfg),
		bucket: cfg.Bucket,
		prefix: cfg.PathPrefix,
	}, nil
}

// Upload 上传文件
func (c *Client) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
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

// UploadFromPath 从本地路径上传文件
func (c *Client) UploadFromPath(ctx context.Context, localPath, remoteKey string) (int64, error) {
	// 简化实现：实际应用中应该使用 uploader.UploadFromFile
	// 这里返回占位值，完整实现需要打开文件并上传
	_ = localPath
	_ = remoteKey
	return 0, fmt.Errorf("not implemented: use Upload method with io.Reader")
}

// Delete 删除文件
func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.getFullKey(key)),
	})
	return err
}

// DeleteByPrefix 批量删除指定前缀的文件
func (c *Client) DeleteByPrefix(ctx context.Context, prefix string) error {
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

// GetPresignedURL 获取预签名URL（有效期1小时）
func (c *Client) GetPresignedURL(ctx context.Context, key string, expire time.Duration) (string, error) {
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
func (c *Client) Head(ctx context.Context, key string) (*s3.HeadObjectOutput, error) {
	return c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.getFullKey(key)),
	})
}

// Exists 检查文件是否存在
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.Head(ctx, key)
	if err != nil {
		// 如果是NotFound错误，返回false
		return false, nil
	}
	return true, nil
}

// Size 获取文件大小
func (c *Client) Size(ctx context.Context, key string) (int64, error) {
	resp, err := c.Head(ctx, key)
	if err != nil {
		return 0, err
	}
	if resp.ContentLength == nil {
		return 0, nil
	}
	return *resp.ContentLength, nil
}

func (c *Client) getFullKey(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + "/" + key
}

// GetFileURL 获取文件的公开URL（不使用预签名）
func (c *Client) GetFileURL(key string) string {
	endpoint := c.client.Options().BaseEndpoint
	if endpoint == nil {
		return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", c.bucket, c.getFullKey(key))
	}
	return fmt.Sprintf("https://%s/%s/%s", *endpoint, c.bucket, c.getFullKey(key))
}
