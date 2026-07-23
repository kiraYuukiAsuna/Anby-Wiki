// Package logging 提供 slog JSON Handler 初始化。
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// New 按级别字符串创建 JSON Handler 的 slog.Logger。
// 无法识别的级别降级为 info。
func New(w io.Writer, level string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: parseLevel(level),
	}))
}

// parseLevel 将配置字符串映射为 slog 级别。
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
