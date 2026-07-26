// Package ratelimit implements a Redis-backed fixed-window request limiter.
//
// The application is the only rate-limiting layer in the current deployment
// topology: no reverse proxy sits in front of the API. Limits are therefore
// enforced here and shared across API replicas through Redis.
//
// Redis holds only short-lived counters. Losing them is acceptable and simply
// resets the current window, which matches ADR-0003: Redis never stores data
// that cannot be discarded.
package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Decision is the outcome of a single limiter consultation.
type Decision struct {
	// Allowed reports whether the request may proceed.
	Allowed bool
	// Limit is the configured ceiling for the window.
	Limit int
	// Remaining is the number of further requests permitted in this window.
	Remaining int
	// RetryAfter is how long the caller should wait. Only set when denied.
	RetryAfter time.Duration
}

// Counter is the minimal Redis surface the limiter needs, so tests can supply
// an in-memory double instead of a live server.
type Counter interface {
	Incr(ctx context.Context, key string) *redis.IntCmd
	PExpire(ctx context.Context, key string, ttl time.Duration) *redis.BoolCmd
}

// Limiter enforces a fixed-window quota per (bucket, client) pair.
type Limiter struct {
	client Counter
	window time.Duration
	// failOpen decides behaviour when Redis is unreachable. Availability of
	// the wiki is preferred over strict enforcement, but the caller is told
	// through the returned error so it can be logged.
	failOpen bool
	now      func() time.Time
}

// New builds a Limiter over the supplied Redis client.
func New(client Counter, window time.Duration) *Limiter {
	if window <= 0 {
		window = time.Minute
	}
	return &Limiter{client: client, window: window, failOpen: true, now: time.Now}
}

// Allow consumes one unit from the (bucket, client) window.
//
// On a Redis failure it returns an allowed Decision together with the error:
// callers should log the error but let the request through, because a cache
// outage must not take down the whole API.
func (l *Limiter) Allow(ctx context.Context, bucket, client string, limit int) (Decision, error) {
	if limit <= 0 {
		return Decision{Allowed: true, Limit: limit, Remaining: 0}, nil
	}
	// Window is encoded in the key. Expiry is aligned to the next boundary so
	// Retry-After never tells callers to wait into the following window.
	now := l.now().UTC()
	windowMillis := l.window.Milliseconds()
	slot := now.UnixMilli() / windowMillis
	untilNextWindow := time.Duration(windowMillis-(now.UnixMilli()%windowMillis)) * time.Millisecond
	key := fmt.Sprintf("ratelimit:%s:%s:%d", bucket, client, slot)

	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return Decision{Allowed: l.failOpen, Limit: limit, Remaining: limit}, fmt.Errorf("ratelimit: incr: %w", err)
	}
	if count == 1 {
		// A small grace margin avoids expiring the counter just before a
		// request at the boundary finishes. The slot key itself changes at
		// the boundary, so the grace does not extend the quota window.
		if err := l.client.PExpire(ctx, key, untilNextWindow+time.Second).Err(); err != nil {
			return Decision{Allowed: true, Limit: limit, Remaining: limit - 1}, fmt.Errorf("ratelimit: pexpire: %w", err)
		}
	}
	if count > int64(limit) {
		return Decision{Allowed: false, Limit: limit, Remaining: 0, RetryAfter: untilNextWindow}, nil
	}
	return Decision{Allowed: true, Limit: limit, Remaining: limit - int(count)}, nil
}

// ClientIP derives the limiter identity for a request.
//
// X-Forwarded-For is honoured only when the direct peer is a configured
// trusted proxy; otherwise any client could forge its own identity and bypass
// the limit entirely.
func ClientIP(r *http.Request, trustedProxies map[string]struct{}) string {
	peerIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peerIP = r.RemoteAddr
	}
	if len(trustedProxies) == 0 {
		return peerIP
	}
	if _, ok := trustedProxies[peerIP]; !ok {
		return peerIP
	}
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return peerIP
	}
	// Right-most untrusted address is the closest non-proxy client.
	parts := strings.Split(forwarded, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" {
			continue
		}
		if net.ParseIP(candidate) == nil {
			continue
		}
		if _, ok := trustedProxies[candidate]; ok {
			continue
		}
		return candidate
	}
	return peerIP
}

// TrustedProxySet converts configured proxy IPs into a lookup set.
func TrustedProxySet(ips []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		if trimmed := strings.TrimSpace(ip); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	return set
}
