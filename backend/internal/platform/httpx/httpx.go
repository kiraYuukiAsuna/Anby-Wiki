// Package httpx 提供 HTTP 层公共设施：请求 ID 上下文存取、
// 契约 Error 模型（contracts/openapi components/schemas/Error）的统一写出。
// 请求 ID 由 cmd/api 的 RequestID 中间件经 WithRequestID 写入上下文，
// 错误响应的 request_id 字段与响应头 X-Request-ID 一致。
package httpx

import (
	"context"
	"encoding/json"
	"net/http"
)

// 错误码，与契约 Error.code 枚举保持一致。
const (
	CodeBadRequest       = "bad_request"
	CodeUnauthorized     = "unauthorized"
	CodeForbidden        = "forbidden"
	CodeNotFound         = "not_found"
	CodeGone             = "gone"
	CodeConflict         = "conflict"
	CodeStaleRevision    = "stale_revision"
	CodeValidationFailed = "validation_failed"
	CodeRateLimited      = "rate_limited"
	CodeInternal         = "internal"
)

type ctxKeyRequestID struct{}

// WithRequestID 把请求 ID 写入上下文（由 RequestID 中间件调用）。
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID{}, id)
}

// RequestIDFrom 从上下文取请求 ID；未设置时返回空串。
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKeyRequestID{}).(string); ok {
		return id
	}
	return ""
}

// Error 契约 Error 模型的响应体。
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Details   map[string]any `json:"details,omitempty"`
}

// WriteJSON 以 JSON 写响应。
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// WriteError 按契约 Error 模型写错误响应，request_id 取自请求上下文。
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	WriteJSON(w, status, Error{
		Code:      code,
		Message:   message,
		RequestID: RequestIDFrom(r.Context()),
	})
}
