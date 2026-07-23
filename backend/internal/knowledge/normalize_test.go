package knowledge

import (
	"errors"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/page"
)

func TestNormalizeKey(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"trim 与小写", "  Anby   Demara ", "anby demara"},
		{"全角空格折叠", "Foo　　Bar", "foo bar"},
		{"NFC 分解形式", "Cafe\u0301", "café"},
		{"CJK 原样保留", "安比·德玛拉", "安比·德玛拉"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeKey(tt.raw)
			if err != nil {
				t.Fatalf("normalizeKey(%q) 出错: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeKey(%q) = %q, 期望 %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeKey_Invalid(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"空串", ""},
		{"纯空白", "  　 "},
		{"超过 255 字符", strings.Repeat("a", 256)},
		{"含控制字符", "foo\x00bar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeKey(tt.raw)
			if !errors.Is(err, ErrInvalidLabel) {
				t.Fatalf("normalizeKey(%q) err = %v, 期望 ErrInvalidLabel", tt.raw, err)
			}
			// 包装链上应能回溯到 page.ErrInvalidTitle（规则复用的证据）。
			if !errors.Is(err, page.ErrInvalidTitle) {
				t.Fatalf("normalizeKey(%q) err = %v, 期望包装 page.ErrInvalidTitle", tt.raw, err)
			}
		})
	}
}

func TestDisplayLabel(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"保留大小写", "  Anby   Demara ", "Anby Demara"},
		{"NFC 合成", "Cafe\u0301", "Café"},
		{"全角空格折叠", "Foo　　Bar", "Foo Bar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := displayLabel(tt.raw)
			if err != nil {
				t.Fatalf("displayLabel(%q) 出错: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("displayLabel(%q) = %q, 期望 %q", tt.raw, got, tt.want)
			}
		})
	}
	if _, err := displayLabel("  "); !errors.Is(err, ErrInvalidLabel) {
		t.Fatalf("displayLabel(空白) err = %v, 期望 ErrInvalidLabel", err)
	}
}

func TestLikePattern(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"anby", "%anby%"},
		{"50%", `%50\%%`},
		{"a_b", `%a\_b%`},
		{`a\b`, `%a\\b%`},
	}
	for _, tt := range tests {
		if got := likePattern(tt.raw); got != tt.want {
			t.Fatalf("likePattern(%q) = %q, 期望 %q", tt.raw, got, tt.want)
		}
	}
}
