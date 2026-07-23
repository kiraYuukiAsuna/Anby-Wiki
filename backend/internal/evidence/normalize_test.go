package evidence_test

import (
	"errors"
	"testing"

	"github.com/anby/wiki/backend/internal/evidence"
)

// TestNormalizeURL 表驱动：大小写、默认端口、fragment、路径 ./..、
// 追踪参数剔除与查询参数排序（纯函数，无 DB）。
func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"scheme/host 小写", "HTTP://Example.COM/Path", "http://example.com/Path"},
		{"路径大小写保留", "http://example.com/A/B", "http://example.com/A/B"},
		{"http 默认端口剔除", "http://example.com:80/a", "http://example.com/a"},
		{"https 默认端口剔除", "https://example.com:443/a", "https://example.com/a"},
		{"非默认端口保留", "http://example.com:8080/a", "http://example.com:8080/a"},
		{"https 带 80 端口保留", "https://example.com:80/a", "https://example.com:80/a"},
		{"fragment 剔除", "http://example.com/a#section-1", "http://example.com/a"},
		{"空 fragment 剔除", "http://example.com/a#", "http://example.com/a"},
		{"路径点段归一", "http://example.com/a/./b/../c", "http://example.com/a/c"},
		{"路径多级上级", "http://example.com/a/b/../../c", "http://example.com/c"},
		{"非根尾部斜杠剔除", "http://example.com/a/b/", "http://example.com/a/b"},
		{"根路径保留斜杠", "http://example.com/", "http://example.com/"},
		{"空路径归为根路径", "http://example.com", "http://example.com/"},
		{"utm 参数剔除", "http://example.com/a?utm_source=x&utm_medium=y&id=1", "http://example.com/a?id=1"},
		{"utm 参数名大小写不敏感", "http://example.com/a?UTM_SOURCE=x&id=1", "http://example.com/a?id=1"},
		{"gclid/fbclid 剔除", "http://example.com/a?gclid=g1&fbclid=f1&id=1", "http://example.com/a?id=1"},
		{"全是追踪参数则查询为空", "http://example.com/a?utm_source=x&gclid=g1", "http://example.com/a"},
		{"查询参数按 key 排序", "http://example.com/a?b=2&a=1", "http://example.com/a?a=1&b=2"},
		{"同 key 值排序", "http://example.com/a?b=2&b=1&a=0", "http://example.com/a?a=0&b=1&b=2"},
		{"组合：大小写+端口+fragment+追踪+排序", "HTTPS://Example.COM:443/A/../B?z=1&utm_campaign=c&a=2#frag", "https://example.com/B?a=2&z=1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evidence.NormalizeURL(tc.raw)
			if err != nil {
				t.Fatalf("NormalizeURL(%q) err = %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeURL(%q) = %q，期望 %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNormalizeURL_Invalid(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"空串", ""},
		{"纯空白", "   "},
		{"无 scheme", "example.com/a"},
		{"非 http/https scheme", "ftp://example.com/a"},
		{"缺少 host", "http:///a"},
		{"非法字符", "http://exa mple.com/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := evidence.NormalizeURL(tc.raw); !errors.Is(err, evidence.ErrInvalidURL) {
				t.Fatalf("NormalizeURL(%q) err = %v，期望 ErrInvalidURL", tc.raw, err)
			}
		})
	}
}
