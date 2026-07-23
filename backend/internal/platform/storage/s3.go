package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Config S3 兼容存储配置（ADR-0004：仅 endpoint/region/bucket/凭据，
// 不出现供应商专有特性）。endpoint 指向 MinIO 或任意 S3 兼容服务。
type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

// S3Store 基于 aws-sdk-go-v2 的 Store 实现。
//
// 注意：截至 M4-T04，本实现只保证编译通过；本地/CI 无 Docker（MinIO 不可用），
// 真实 S3 行为验证由 S3_TEST_ENDPOINT 门控的集成测试承担（见 s3_test.go），
// 默认环境 skip——视为遗留验证项，待有 Docker 的环境补验。
type S3Store struct {
	client *s3.Client
	bucket string
}

// NewS3Store 装配 S3Store。MinIO 等 S3 兼容实现需要 path-style 寻址。
func NewS3Store(cfg S3Config) *S3Store {
	client := s3.New(s3.Options{
		Region:       cfg.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		BaseEndpoint: aws.String(cfg.Endpoint),
		UsePathStyle: true,
	})
	return &S3Store{client: client, bucket: cfg.Bucket}
}

// Put 实现 Store。
func (s *S3Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          r,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("storage: S3 Put 失败 key=%q: %w", key, err)
	}
	return nil
}

// Get 实现 Store。
func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage: S3 Get 失败 key=%q: %w", key, err)
	}
	return out.Body, nil
}

// Head 实现 Store。
func (s *S3Store) Head(ctx context.Context, key string) (ObjectMeta, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return ObjectMeta{}, ErrNotFound
		}
		return ObjectMeta{}, fmt.Errorf("storage: S3 Head 失败 key=%q: %w", key, err)
	}
	return ObjectMeta{
		Key:         key,
		Size:        aws.ToInt64(out.ContentLength),
		ContentType: aws.ToString(out.ContentType),
	}, nil
}

// Delete 实现 Store（S3 DeleteObject 本身幂等：删不存在的 key 返回成功）。
func (s *S3Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: S3 Delete 失败 key=%q: %w", key, err)
	}
	return nil
}

// isNotFound 判定 S3 错误是否为“对象不存在”：
// 类型断言（NoSuchKey/NotFound）+ smithy 错误码兜底（MinIO 等实现的码值差异）。
func isNotFound(err error) bool {
	var noSuchKey *types.NoSuchKey
	var notFound *types.NotFound
	if errors.As(err, &noSuchKey) || errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	return false
}
