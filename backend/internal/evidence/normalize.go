package evidence

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"sort"
	"strings"
)

// scheme 的默认端口：规范化时剔除。
var defaultPorts = map[string]string{
	"http":  "80",
	"https": "443",
}

// isTrackingParam 常见追踪参数：utm_ 前缀，以及精确匹配的 gclid/fbclid
// （参数名大小写不敏感）。
func isTrackingParam(key string) bool {
	k := strings.ToLower(key)
	return strings.HasPrefix(k, "utm_") || k == "gclid" || k == "fbclid"
}

// NormalizeURL 规范化外部 URL（纯函数，无 DB 依赖）。规则：
//   - 仅接受 http/https 绝对 URL；
//   - scheme 与 host 小写，剔除默认端口（http:80 / https:443）；
//   - 剔除 fragment（#...）；
//   - 路径按 path.Clean 去 ./.. 段；根路径保留 "/"，其余尾部斜杠剔除；
//   - 查询参数剔除追踪参数（utm_*/gclid/fbclid），剩余按 key 排序，
//     同 key 的值按字典序排序（保证输出确定）。
//
// 输入非法返回 ErrInvalidURL。规范化结果用作 external_resource.normalized_url
// 的查重键（设计 §7.1、§8：去追踪参数等变化不应产生新资源行）。
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("%w: 解析失败: %v", ErrInvalidURL, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%w: scheme=%q（仅支持 http/https）", ErrInvalidURL, u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("%w: 缺少 host", ErrInvalidURL)
	}
	// 先取端口：u.Host 被小写 host 覆盖后 u.Port() 恒为空，
	// 非默认端口会被误剔除。
	port := u.Port()
	u.Scheme = scheme
	u.Host = host
	if strings.Contains(host, ":") {
		// IPv6 字面量需要方括号。
		u.Host = "[" + host + "]"
	}
	if port != "" && port != defaultPorts[scheme] {
		u.Host = net.JoinHostPort(host, port)
	}
	u.Fragment = ""
	u.RawFragment = ""

	// path.Clean 去 ./.. 段与非根尾部斜杠；空路径归为根路径 "/"。
	p := path.Clean(u.Path)
	if p == "." || p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u.Path = p
	u.RawPath = ""

	q := u.Query()
	kept := url.Values{}
	for key, vals := range q {
		if isTrackingParam(key) {
			continue
		}
		for _, v := range vals {
			kept.Add(key, v)
		}
	}
	for key := range kept {
		sort.Strings(kept[key])
	}
	u.RawQuery = kept.Encode()

	return u.String(), nil
}
