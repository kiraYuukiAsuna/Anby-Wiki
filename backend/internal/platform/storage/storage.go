// Package storage 定义对象存储窄接口（ADR-0004）。
//
// 业务代码只依赖本包的 Store 接口，不直接依赖 AWS SDK 类型。
// 对象键按内容寻址：{env}/{domain}/{content_hash前2位}/{content_hash}
// （ContentKey 构造），同内容天然去重。
//
// 实现：
//   - S3Store：aws-sdk-go-v2，endpoint 可配（MinIO/任意 S3 兼容服务）；
//   - Fake：内存实现，供全部单元/集成测试使用（本机无 Docker 时
//     真实 MinIO 验证以 S3_TEST_ENDPOINT 环境变量门控，见 s3_test.go）。
//
// PresignGet 暂缓，M6 需要时再加。
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrNotFound 对象不存在（Head/Get 未命中）。
var ErrNotFound = errors.New("storage: 对象不存在")

// Store 对象存储窄接口（ADR-0004）。
type Store interface {
	// Put 以 key 写入 size 字节内容；同 key 覆盖写（内容寻址下同 key 即同内容，幂等）。
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get 读取对象内容；未命中返回 ErrNotFound。调用方负责关闭返回的 ReadCloser。
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// Head 查询对象元数据；未命中返回 ErrNotFound。
	Head(ctx context.Context, key string) (ObjectMeta, error)
	// Delete 删除对象；删除不存在的 key 视为成功（幂等）。
	Delete(ctx context.Context, key string) error
}

// ObjectMeta 对象元数据。
type ObjectMeta struct {
	Key         string
	Size        int64
	ContentType string
}

// ContentKey 按 ADR-0004 约定构造内容寻址键：
//
//	{env}/{domain}/{content_hash前2位}/{content_hash}
//
// contentHash 为十六进制摘要（本仓库用 SHA-256，64 位小写 hex）；
// 不足 2 位或含非 hex 字符视为非法。env/domain 不允许为空或含 "/"。
func ContentKey(env, domain, contentHash string) (string, error) {
	if strings.TrimSpace(env) == "" || strings.Contains(env, "/") {
		return "", fmt.Errorf("storage: env 非法: %q", env)
	}
	if strings.TrimSpace(domain) == "" || strings.Contains(domain, "/") {
		return "", fmt.Errorf("storage: domain 非法: %q", domain)
	}
	if len(contentHash) < 2 {
		return "", fmt.Errorf("storage: content_hash 过短: %q", contentHash)
	}
	for _, c := range contentHash {
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		if !isDigit && !isLowerHex {
			return "", fmt.Errorf("storage: content_hash 非小写 hex: %q", contentHash)
		}
	}
	return env + "/" + domain + "/" + contentHash[:2] + "/" + contentHash, nil
}
