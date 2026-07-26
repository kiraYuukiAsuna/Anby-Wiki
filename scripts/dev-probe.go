//go:build ignore

// dev-probe 是 scripts/dev.sh 使用的依赖可达性探测器。
//
// 只依赖标准库，因此可以用 `go run scripts/dev-probe.go` 在任何模块上下文外执行。
// 从 GO_PROBE_TARGET 读取一个连接串（postgres:// / redis:// / http(s)://），
// 解析出 host:port 后做一次 TCP 拨号；可达返回 0，否则返回 1。
package main

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

// defaultPorts 为常见 scheme 补全缺省端口。
var defaultPorts = map[string]string{
	"postgres":   "5432",
	"postgresql": "5432",
	"redis":      "6379",
	"rediss":     "6379",
	"http":       "80",
	"https":      "443",
}

func main() {
	target := strings.TrimSpace(os.Getenv("GO_PROBE_TARGET"))
	if target == "" {
		fmt.Fprintln(os.Stderr, "dev-probe: GO_PROBE_TARGET is empty")
		os.Exit(1)
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" {
		fmt.Fprintln(os.Stderr, "dev-probe: cannot parse target URL")
		os.Exit(1)
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		port = defaultPorts[parsed.Scheme]
	}
	if port == "" {
		fmt.Fprintf(os.Stderr, "dev-probe: no port for scheme %q\n", parsed.Scheme)
		os.Exit(1)
	}
	address := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dev-probe: %s unreachable: %v\n", address, err)
		os.Exit(1)
	}
	_ = conn.Close()
}
