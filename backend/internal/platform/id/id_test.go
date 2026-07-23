package id

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerator_Format(t *testing.T) {
	g := NewGenerator()
	id, err := g.New()
	if err != nil {
		t.Fatalf("New() 返回错误: %v", err)
	}
	// 字符串可被 google/uuid 解析，且版本为 7。
	parsed, err := uuid.Parse(id.String())
	if err != nil {
		t.Fatalf("生成结果无法解析: %v", err)
	}
	if parsed.Version() != 7 {
		t.Errorf("版本 = %d, 期望 7", parsed.Version())
	}
}

func TestGenerator_Uniqueness(t *testing.T) {
	g := NewGenerator()
	seen := make(map[uuid.UUID]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id, err := g.New()
		if err != nil {
			t.Fatalf("New() 返回错误: %v", err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("第 %d 次生成出现重复 ID: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestGenerator_TimeOrdered(t *testing.T) {
	// 注入递增时钟，验证生成结果随时间单调有序。
	base := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	var step int64
	g := NewGeneratorWithClock(func() time.Time {
		step++
		return base.Add(time.Duration(step) * time.Millisecond)
	})

	prev, err := g.New()
	if err != nil {
		t.Fatalf("New() 返回错误: %v", err)
	}
	for i := 0; i < 100; i++ {
		next, err := g.New()
		if err != nil {
			t.Fatalf("New() 返回错误: %v", err)
		}
		// 时间戳递增时，字符串字典序也应递增（V7 时间戳在高位）。
		if next.String() <= prev.String() {
			t.Fatalf("第 %d 次生成未保持时间有序: %s <= %s", i, next, prev)
		}
		prev = next
	}
}

func TestTimestampOf(t *testing.T) {
	fixed := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	g := NewGeneratorWithClock(func() time.Time { return fixed })
	id, err := g.New()
	if err != nil {
		t.Fatalf("New() 返回错误: %v", err)
	}
	if got := TimestampOf(id); !got.Equal(fixed) {
		t.Errorf("TimestampOf() = %v, 期望 %v", got, fixed)
	}
}
