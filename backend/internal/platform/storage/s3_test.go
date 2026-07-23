package storage_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/platform/storage"
)

// TestS3Store_RoundTrip 真实 S3 兼容服务（MinIO）冒烟。
//
// 本机/CI 无 Docker 时不强制运行：仅当设置 S3_TEST_ENDPOINT 才执行，否则 skip。
// 桶名/凭据默认取 MinIO 本地开发惯例值，可用 S3_TEST_BUCKET /
// S3_TEST_ACCESS_KEY / S3_TEST_SECRET_KEY 覆盖。
//
// 遗留：S3Store 在 M4-T04 只保证编译通过，真实行为验证依赖本测试在
// 有 Docker 的环境执行（详见 s3.go 注释）。
func TestS3Store_RoundTrip(t *testing.T) {
	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_TEST_ENDPOINT 未设置，跳过真实 S3 集成测试（本机无 Docker/MinIO）")
	}
	bucket := os.Getenv("S3_TEST_BUCKET")
	if bucket == "" {
		bucket = "wiki-dev"
	}
	accessKey := os.Getenv("S3_TEST_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "minioadmin"
	}
	secretKey := os.Getenv("S3_TEST_SECRET_KEY")
	if secretKey == "" {
		secretKey = "minioadmin"
	}

	store := storage.NewS3Store(storage.S3Config{
		Endpoint:  endpoint,
		Region:    "us-east-1",
		Bucket:    bucket,
		AccessKey: accessKey,
		SecretKey: secretKey,
	})
	ctx := context.Background()
	key := "test/storage/" + strings.Repeat("ef", 32)

	if _, err := store.Head(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("未上传时 Head err = %v, 期望 ErrNotFound", err)
	}

	content := "s3 round trip"
	if err := store.Put(ctx, key, strings.NewReader(content), int64(len(content)), "text/plain"); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(context.Background(), key) })

	meta, err := store.Head(ctx, key)
	if err != nil {
		t.Fatalf("Head 失败: %v", err)
	}
	if meta.Size != int64(len(content)) {
		t.Fatalf("meta.Size = %d, 期望 %d", meta.Size, len(content))
	}

	rc, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("读取内容失败: %v", err)
	}
	if string(got) != content {
		t.Fatalf("内容 = %q, 期望 %q", got, content)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	if _, err := store.Head(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("删除后 Head err = %v, 期望 ErrNotFound", err)
	}
}
