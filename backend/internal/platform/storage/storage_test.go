package storage_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/platform/storage"
)

func TestContentKey(t *testing.T) {
	hash := strings.Repeat("ab", 32) // 64 位小写 hex
	key, err := storage.ContentKey("development", "asset", hash)
	if err != nil {
		t.Fatalf("ContentKey 失败: %v", err)
	}
	want := "development/asset/ab/" + hash
	if key != want {
		t.Fatalf("key = %q, 期望 %q", key, want)
	}
}

func TestContentKey_Invalid(t *testing.T) {
	cases := []struct{ env, domain, hash string }{
		{"", "asset", strings.Repeat("ab", 32)},
		{"dev/elopment", "asset", strings.Repeat("ab", 32)},
		{"development", "", strings.Repeat("ab", 32)},
		{"development", "ass/et", strings.Repeat("ab", 32)},
		{"development", "asset", "a"},                             // 过短
		{"development", "asset", "zz" + strings.Repeat("ab", 31)}, // 非 hex
		{"development", "asset", "AB" + strings.Repeat("ab", 31)}, // 大写 hex
	}
	for _, c := range cases {
		if _, err := storage.ContentKey(c.env, c.domain, c.hash); err == nil {
			t.Fatalf("ContentKey(%q, %q, %q) 应报错", c.env, c.domain, c.hash)
		}
	}
}

func TestFake_PutGetHeadDelete(t *testing.T) {
	ctx := context.Background()
	store := storage.NewFake()
	key := "development/asset/ab/" + strings.Repeat("ab", 32)

	// 未命中：Head/Get 返回 ErrNotFound。
	if _, err := store.Head(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("空 Fake Head err = %v, 期望 ErrNotFound", err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("空 Fake Get err = %v, 期望 ErrNotFound", err)
	}

	content := "hello evidence"
	if err := store.Put(ctx, key, strings.NewReader(content), int64(len(content)), "text/plain"); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}

	meta, err := store.Head(ctx, key)
	if err != nil {
		t.Fatalf("Head 失败: %v", err)
	}
	if meta.Key != key || meta.Size != int64(len(content)) || meta.ContentType != "text/plain" {
		t.Fatalf("meta 异常: %+v", meta)
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
	// Delete 幂等：删不存在的 key 不报错。
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("重复 Delete 应幂等: %v", err)
	}
}

func TestFake_PutOverwriteAndCount(t *testing.T) {
	ctx := context.Background()
	store := storage.NewFake()
	key := "development/asset/cd/" + strings.Repeat("cd", 32)

	for i := 0; i < 3; i++ {
		if err := store.Put(ctx, key, strings.NewReader("v"), 1, "text/plain"); err != nil {
			t.Fatalf("Put #%d 失败: %v", i, err)
		}
	}
	if n := store.PutCount(); n != 3 {
		t.Fatalf("PutCount = %d, 期望 3", n)
	}
	if keys := store.Keys(); len(keys) != 1 || keys[0] != key {
		t.Fatalf("Keys = %v, 期望仅 %q", keys, key)
	}
}
