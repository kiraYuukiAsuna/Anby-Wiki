package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/platform/observability"
)

// requestIDHeader 透传/回写请求 ID 的响应头。
const requestIDHeader = "X-Request-ID"

// RequestID 中间件：透传或生成 X-Request-ID，写入响应头与请求上下文。
// 上下文键归 platform/httpx 所有，错误响应的 request_id 字段由此读取。
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(httpx.WithRequestID(r.Context(), id)))
	})
}

// newRequestID 生成随机请求 ID（16 字节十六进制）。
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 随机源不可用时退化为时间戳，保证不中断请求。
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}

// requestIDFrom 从上下文取请求 ID。
func requestIDFrom(ctx context.Context) string {
	return httpx.RequestIDFrom(ctx)
}

// Authentication resolves a server-side session or an explicitly enabled
// development actor header and stores only identity in context.
func Authentication(authenticator *authdomain.Authenticator, testHeaderFallback bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authenticator != nil {
				principal, ok, err := authenticator.Authenticate(r.Context(), r)
				if err != nil {
					httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal, "认证服务暂时不可用")
					return
				}
				if ok {
					r = r.WithContext(authdomain.WithPrincipal(r.Context(), principal))
				}
			} else if testHeaderFallback {
				// Existing handler tests have no auth database assembly. This
				// branch is unreachable from main because it always sets Env.
				if actorID, err := uuid.Parse(r.Header.Get(authdomain.DevActorHeader)); err == nil {
					r = r.WithContext(authdomain.WithPrincipal(r.Context(), authdomain.Principal{
						ActorID: actorID,
						Method:  "test_header",
					}))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Recoverer 中间件：恢复 panic，返回 500 并记录日志。
func Recoverer(logger *slog.Logger, options ...any) func(http.Handler) http.Handler {
	var service string
	var metrics *observability.Metrics
	for _, option := range options {
		switch value := option.(type) {
		case string:
			service = value
		case *observability.Metrics:
			metrics = value
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if metrics != nil {
						metrics.ObservePanic(service, r)
					}
					logger.Error("panic recovered",
						slog.Any("panic", rec),
						slog.String("request_id", requestIDFrom(r.Context())),
					)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// AccessLog 中间件：记录每次访问的方法、路径、状态码与耗时。
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.Info("http access",
				slog.String("request_id", requestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}
