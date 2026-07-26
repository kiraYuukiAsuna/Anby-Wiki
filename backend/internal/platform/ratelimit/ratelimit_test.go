package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// fakeCounter is an in-memory Counter double so limiter behaviour can be
// asserted without a live Redis server.
type fakeCounter struct {
	values map[string]int64
	ttls   map[string]time.Duration
	err    error
}

func newFakeCounter() *fakeCounter {
	return &fakeCounter{values: map[string]int64{}, ttls: map[string]time.Duration{}}
}

func (f *fakeCounter) Incr(ctx context.Context, key string) *redis.IntCmd {
	cmd := redis.NewIntCmd(ctx)
	if f.err != nil {
		cmd.SetErr(f.err)
		return cmd
	}
	f.values[key]++
	cmd.SetVal(f.values[key])
	return cmd
}

func (f *fakeCounter) PExpire(ctx context.Context, key string, ttl time.Duration) *redis.BoolCmd {
	cmd := redis.NewBoolCmd(ctx)
	if f.err != nil {
		cmd.SetErr(f.err)
		return cmd
	}
	f.ttls[key] = ttl
	cmd.SetVal(true)
	return cmd
}

func TestAllowRejectsBeyondLimitAndReportsRetryAfter(t *testing.T) {
	counter := newFakeCounter()
	limiter := New(counter, time.Minute)
	limiter.now = func() time.Time {
		return time.Date(2026, 7, 25, 10, 0, 30, 0, time.UTC)
	}
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		decision, err := limiter.Allow(ctx, "auth", "10.0.0.1", 3)
		if err != nil || !decision.Allowed {
			t.Fatalf("request %d allowed=%v err=%v", i, decision.Allowed, err)
		}
		if decision.Remaining != 3-i {
			t.Fatalf("request %d remaining=%d want %d", i, decision.Remaining, 3-i)
		}
	}
	decision, err := limiter.Allow(ctx, "auth", "10.0.0.1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatal("fourth request must be denied")
	}
	if decision.RetryAfter <= 0 {
		t.Fatalf("denied decision must report RetryAfter, got %s", decision.RetryAfter)
	}
	if decision.RetryAfter > 30*time.Second {
		t.Fatalf("RetryAfter=%s must end at the current window boundary", decision.RetryAfter)
	}
}

func TestAllowIsolatesClientsAndBuckets(t *testing.T) {
	limiter := New(newFakeCounter(), time.Minute)
	ctx := context.Background()

	if _, err := limiter.Allow(ctx, "auth", "10.0.0.1", 1); err != nil {
		t.Fatal(err)
	}
	// A different client must retain its own budget.
	other, err := limiter.Allow(ctx, "auth", "10.0.0.2", 1)
	if err != nil || !other.Allowed {
		t.Fatalf("other client allowed=%v err=%v", other.Allowed, err)
	}
	// A different bucket for the same client is also independent.
	general, err := limiter.Allow(ctx, "general", "10.0.0.1", 1)
	if err != nil || !general.Allowed {
		t.Fatalf("other bucket allowed=%v err=%v", general.Allowed, err)
	}
}

func TestAllowFailsOpenWhenRedisUnavailable(t *testing.T) {
	counter := newFakeCounter()
	counter.err = errors.New("redis down")
	limiter := New(counter, time.Minute)

	decision, err := limiter.Allow(context.Background(), "auth", "10.0.0.1", 1)
	if err == nil {
		t.Fatal("Redis failure must be reported to the caller")
	}
	if !decision.Allowed {
		t.Fatal("limiter must fail open so a cache outage cannot take the API down")
	}
}

func TestAllowStartsNewWindow(t *testing.T) {
	counter := newFakeCounter()
	limiter := New(counter, time.Minute)
	current := time.Date(2026, 7, 25, 10, 0, 30, 0, time.UTC)
	limiter.now = func() time.Time { return current }
	ctx := context.Background()

	if _, err := limiter.Allow(ctx, "auth", "10.0.0.1", 1); err != nil {
		t.Fatal(err)
	}
	denied, err := limiter.Allow(ctx, "auth", "10.0.0.1", 1)
	if err != nil || denied.Allowed {
		t.Fatalf("second request in window allowed=%v err=%v", denied.Allowed, err)
	}
	// Crossing the window boundary resets the quota.
	current = current.Add(time.Minute)
	allowed, err := limiter.Allow(ctx, "auth", "10.0.0.1", 1)
	if err != nil || !allowed.Allowed {
		t.Fatalf("next window allowed=%v err=%v", allowed.Allowed, err)
	}
}

func TestClientIPIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/pages", nil)
	request.RemoteAddr = "203.0.113.9:1234"
	request.Header.Set("X-Forwarded-For", "1.2.3.4")

	// No trusted proxies: a client must not be able to choose its own bucket.
	if got := ClientIP(request, nil); got != "203.0.113.9" {
		t.Fatalf("ClientIP=%q want socket peer", got)
	}
	// Peer is not a configured proxy, so the header is still ignored.
	if got := ClientIP(request, TrustedProxySet([]string{"10.0.0.1"})); got != "203.0.113.9" {
		t.Fatalf("ClientIP=%q want socket peer for untrusted peer", got)
	}
}

func TestClientIPHonoursForwardedHeaderFromTrustedProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/pages", nil)
	request.RemoteAddr = "10.0.0.1:4321"
	request.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")

	got := ClientIP(request, TrustedProxySet([]string{"10.0.0.1"}))
	if got != "1.2.3.4" {
		t.Fatalf("ClientIP=%q want the closest untrusted client", got)
	}
}
