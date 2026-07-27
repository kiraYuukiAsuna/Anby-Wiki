package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/httpx"
	"github.com/anby/wiki/backend/internal/platform/observability"
	"github.com/anby/wiki/backend/internal/platform/ratelimit"
)

const (
	defaultRequestBodyLimit int64 = 2 << 20
	uploadRequestBodyLimit  int64 = 11 << 20
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

// RateLimitConfig declares the per-bucket quotas enforced by RateLimit.
type RateLimitConfig struct {
	GeneralPerMinute int
	AuthPerMinute    int
	UploadPerMinute  int
	TrustedProxies   map[string]struct{}
}

// bucketFor classifies a request path into a quota bucket. Auth and upload
// endpoints get far tighter quotas than ordinary reads and writes.
func bucketFor(path string) (string, bool) {
	switch {
	case path == "/api/v1/import-jobs/uploads":
		return "upload", true
	case strings.HasPrefix(path, "/api/v1/auth/"):
		return "auth", true
	case strings.HasPrefix(path, "/api/"):
		return "general", true
	default:
		// Probes and /metrics are never limited.
		return "", false
	}
}

// RateLimit enforces Redis-backed quotas. It replaces the reverse-proxy
// limiter that previously fronted the API, so it is the only protection
// against request floods and credential stuffing.
//
// A Redis outage fails open: the request proceeds and the failure is logged,
// because losing the cache must not take the wiki offline.
func RateLimit(limiter *ratelimit.Limiter, cfg RateLimitConfig, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bucket, limited := bucketFor(r.URL.Path)
			if !limited {
				next.ServeHTTP(w, r)
				return
			}
			limit := cfg.GeneralPerMinute
			switch bucket {
			case "auth":
				limit = cfg.AuthPerMinute
			case "upload":
				limit = cfg.UploadPerMinute
			}
			client := ratelimit.ClientIP(r, cfg.TrustedProxies)
			decision, err := limiter.Allow(r.Context(), bucket, client, limit)
			if err != nil {
				// Never log the client identity alongside a failure at info
				// level; the bucket alone is enough to diagnose.
				logger.Warn("rate limiter unavailable, allowing request",
					slog.String("bucket", bucket),
					slog.String("request_id", requestIDFrom(r.Context())),
					slog.Any("error", err),
				)
			}
			if decision.Limit > 0 {
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(decision.Limit))
				w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(decision.Remaining))
			}
			if !decision.Allowed {
				retry := int(decision.RetryAfter.Seconds())
				if retry < 1 {
					retry = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retry))
				httpx.WriteError(w, r, http.StatusTooManyRequests, httpx.CodeRateLimited,
					"请求过于频繁，请稍后再试")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders applies the baseline response headers that the removed
// reverse proxy used to add. The API serves JSON only, so the policy is
// deliberately strict.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}

// RequestBodyLimit retains the request-size boundary that previously lived in
// the reverse proxy. Uploads get room for the 10 MiB file plus multipart
// framing; every other request is capped at 2 MiB.
func RequestBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := defaultRequestBodyLimit
		if r.URL.Path == "/api/v1/import-jobs/uploads" {
			limit = uploadRequestBodyLimit
		}
		if r.ContentLength > limit {
			httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest,
				"请求体超过大小限制")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

// StripSpoofableAuthHeaders removes client-supplied identity headers that the
// removed reverse proxy used to clear. Without this, a request could present
// X-Actor-ID directly to the API whenever the development header path is on.
func StripSpoofableAuthHeaders(allowDevActorHeader bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, name := range []string{
				"X-Authenticated-User", "X-Auth-Request-User", "X-Remote-User",
			} {
				r.Header.Del(name)
			}
			if !allowDevActorHeader {
				r.Header.Del("X-Actor-ID")
			}
			next.ServeHTTP(w, r)
		})
	}
}
