package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/anby/wiki/backend/internal/platform/ratelimit"
)

type rateLimitCounter struct {
	counts map[string]int64
}

func (c *rateLimitCounter) Incr(_ context.Context, key string) *redis.IntCmd {
	c.counts[key]++
	return redis.NewIntResult(c.counts[key], nil)
}

func (c *rateLimitCounter) PExpire(_ context.Context, _ string, _ time.Duration) *redis.BoolCmd {
	return redis.NewBoolResult(true, nil)
}

func TestRateLimitSeparatesCredentialsAndQueries(t *testing.T) {
	// A long fixed window keeps this sequence away from minute boundaries.
	limiter := ratelimit.New(&rateLimitCounter{counts: map[string]int64{}}, time.Hour)
	handler := RateLimit(limiter, RateLimitConfig{
		GeneralPerMinute: 100, AuthPerMinute: 3, UploadPerMinute: 1,
		TrustedProxies: ratelimit.TrustedProxySet([]string{"172.25.0.1", "172.25.0.9"}),
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := func(method, path, peer, client string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		r.RemoteAddr = peer + ":1234"
		r.Header.Set("X-Forwarded-For", client)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, w.Code, want, w.Body.String())
		}
		if w.Header().Get("Set-Cookie") != "" {
			t.Fatal("limiter must not modify the session cookie")
		}
		return w
	}
	for range 12 {
		request(http.MethodGet, "/api/v1/auth/session", "172.25.0.1", "203.0.113.7", http.StatusNoContent)
	}
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/register", "/api/v1/auth/cli/exchange"} {
		w := request(http.MethodPost, path, "172.25.0.1", "203.0.113.7", http.StatusNoContent)
		if w.Header().Get("X-RateLimit-Limit") != "3" {
			t.Fatal("credential attempt did not use auth quota")
		}
	}
	w := request(http.MethodPost, "/api/v1/auth/login", "172.25.0.9", "203.0.113.7, 172.25.0.1", http.StatusTooManyRequests)
	if seconds, err := strconv.Atoi(w.Header().Get("Retry-After")); err != nil || seconds < 1 || seconds > 3600 {
		t.Fatalf("invalid Retry-After=%q", w.Header().Get("Retry-After"))
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/auth/session"},
		{http.MethodGet, "/api/v1/auth/cli/tokens"},
		{http.MethodGet, "/api/v1/auth/cli/actors"},
		{http.MethodPost, "/api/v1/auth/cli/codes"},
		{http.MethodDelete, "/api/v1/auth/cli/tokens/token-id"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/auth/login"},
	} {
		request(route.method, route.path, "172.25.0.1", "203.0.113.7", http.StatusNoContent)
	}
	request(http.MethodPost, "/api/v1/auth/login", "172.25.0.1", "203.0.113.8", http.StatusNoContent)
	request(http.MethodPost, "/api/v1/import-jobs/uploads", "172.25.0.1", "203.0.113.7", http.StatusNoContent)
	request(http.MethodPost, "/api/v1/import-jobs/uploads", "172.25.0.1", "203.0.113.7", http.StatusTooManyRequests)
	request(http.MethodGet, "/readyz", "172.25.0.1", "203.0.113.7", http.StatusNoContent)
}
