package page

import (
	"strings"
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"小写化", "Hello World", "hello world"},
		{"去首尾空白", "  Foo Bar  ", "foo bar"},
		{"内部连续空格折叠", "Foo    Bar", "foo bar"},
		{"全角空格折叠", "Foo　　Bar", "foo bar"},
		{"NBSP 折叠", "Foo Bar", "foo bar"},
		{"Tab 换行折叠", "Foo\t\n Bar", "foo bar"},
		{"混合大小写与空白", "  Anby  DEMARA ", "anby demara"},
		{"NFC 分解形式", "Café", "café"}, // e + U+0301 → NFC é 后小写
		{"NFC 已组合形式", "Café", "café"},
		{"中文标题不变", "安比·德玛拉", "安比·德玛拉"},
		{"德文大写小写化", "STRASSE", "strasse"},
		{"恰好 255 字符", strings.Repeat("a", 255), strings.Repeat("a", 255)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeTitle(tt.raw)
			if err != nil {
				t.Fatalf("NormalizeTitle(%q) 返回错误: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeTitle(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeTitle_Invalid(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"空字符串", ""},
		{"纯空格", "   "},
		{"纯全角空格", "　　　"},
		{"纯 Tab 换行", "\t\n "},
		{"含 NUL 控制字符", "Foo\x00Bar"},
		{"含 BEL 控制字符", "Foo\x07Bar"},
		{"含 DEL 控制字符", "Foo\x7fBar"},
		{"超过 255 字符", strings.Repeat("a", 256)},
		{"多字节字符超过 255 码点", strings.Repeat("安", 256)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeTitle(tt.raw)
			if err == nil {
				t.Fatalf("NormalizeTitle(%q) = %q, 期望 ErrInvalidTitle", tt.raw, got)
			}
			if got != "" {
				t.Fatalf("出错时应返回空串, got %q", got)
			}
		})
	}
}

func TestDisplayTitle(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"保留大小写", "  Hello   World ", "Hello World"},
		{"保留分解形式不做 NFC", "Café", "Café"},
		{"全角空格折叠", "安比　　德玛拉", "安比 德玛拉"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DisplayTitle(tt.raw)
			if err != nil {
				t.Fatalf("DisplayTitle(%q) 返回错误: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("DisplayTitle(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestDisplayTitle_Invalid(t *testing.T) {
	for _, raw := range []string{"", "  ", "Foo\x00Bar", strings.Repeat("a", 256)} {
		if _, err := DisplayTitle(raw); err == nil {
			t.Fatalf("DisplayTitle(%q) 期望错误", raw)
		}
	}
}
