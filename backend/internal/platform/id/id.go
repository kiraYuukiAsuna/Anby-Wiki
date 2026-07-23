// Package id 提供 UUIDv7 生成封装，时钟可注入以便测试。
package id

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Generator UUIDv7 生成器。
// google/uuid 的 NewV7 使用内置真实时钟，为支持时钟注入，
// 这里在 google/uuid 的 UUID 类型上手工填充 RFC 9562 定义的 V7 字节布局。
type Generator struct {
	now func() time.Time
}

// NewGenerator 创建使用真实时钟的生成器。
func NewGenerator() *Generator {
	return &Generator{now: time.Now}
}

// NewGeneratorWithClock 创建使用注入时钟的生成器（用于测试）。
func NewGeneratorWithClock(now func() time.Time) *Generator {
	return &Generator{now: now}
}

// New 生成一个 UUIDv7：48 位毫秒时间戳 + 版本 + 74 位随机数。
func (g *Generator) New() (uuid.UUID, error) {
	var id uuid.UUID
	ms := uint64(g.now().UnixMilli())
	// 前 48 位为大端毫秒时间戳。
	id[0] = byte(ms >> 40)
	id[1] = byte(ms >> 32)
	id[2] = byte(ms >> 24)
	id[3] = byte(ms >> 16)
	id[4] = byte(ms >> 8)
	id[5] = byte(ms)
	// 后 10 字节由加密随机数填充。
	if _, err := rand.Read(id[6:]); err != nil {
		return uuid.Nil, fmt.Errorf("id: 生成随机数失败: %w", err)
	}
	// 设置版本（7）与 RFC 4122 变体位。
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id, nil
}

// NewString 生成 UUIDv7 字符串。
func (g *Generator) NewString() (string, error) {
	id, err := g.New()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// TimestampOf 提取 UUIDv7 中编码的毫秒时间戳。
func TimestampOf(id uuid.UUID) time.Time {
	ms := uint64(id[0])<<40 | uint64(id[1])<<32 | uint64(id[2])<<24 |
		uint64(id[3])<<16 | uint64(id[4])<<8 | uint64(id[5])
	return time.UnixMilli(int64(ms)).UTC()
}
