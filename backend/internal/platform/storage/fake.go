package storage

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// Fake 内存 Store 实现：map + 锁，供全部单元/集成测试使用。
// 语义与 S3 对齐：Put 覆盖写、Get/Head 未命中返回 ErrNotFound、Delete 幂等。
type Fake struct {
	mu      sync.Mutex
	objects map[string]fakeObject
	puts    int
}

type fakeObject struct {
	content     []byte
	contentType string
}

// NewFake 创建空的内存 Store。
func NewFake() *Fake {
	return &Fake{objects: make(map[string]fakeObject)}
}

// Put 实现 Store。
func (f *Fake) Put(_ context.Context, key string, r io.Reader, _ int64, contentType string) error {
	content, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = fakeObject{content: content, contentType: contentType}
	f.puts++
	return nil
}

// Get 实现 Store。
func (f *Fake) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	obj, ok := f.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(obj.content)), nil
}

// Head 实现 Store。
func (f *Fake) Head(_ context.Context, key string) (ObjectMeta, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	obj, ok := f.objects[key]
	if !ok {
		return ObjectMeta{}, ErrNotFound
	}
	return ObjectMeta{Key: key, Size: int64(len(obj.content)), ContentType: obj.contentType}, nil
}

// Delete 实现 Store（幂等）。
func (f *Fake) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	return nil
}

// PutCount 返回累计 Put 调用次数（断言去重语义：重复上传不应重复 Put）。
func (f *Fake) PutCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.puts
}

// Keys 返回当前全部对象键（无序），供测试断言存储布局。
func (f *Fake) Keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.objects))
	for k := range f.objects {
		keys = append(keys, k)
	}
	return keys
}
