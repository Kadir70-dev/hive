package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

var rule = Rule{Name: "t", Limit: 3, Window: time.Minute}

func newRedis(t *testing.T) (*miniredis.Miniredis, *Limiter) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { rdb.Close() })
	return mr, New(rdb, nil)
}

func TestRedisLimitAndExpiry(t *testing.T) {
	mr, l := newRedis(t)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		if r := l.Hit(ctx, rule, "k"); !r.Allowed || r.Degraded {
			t.Fatalf("hit %d: %+v", i, r)
		}
	}
	r := l.Hit(ctx, rule, "k")
	if r.Allowed || r.RetryAfter <= 0 || r.RetryAfter > time.Minute {
		t.Fatalf("4th hit should be limited with retry info: %+v", r)
	}
	if p := l.Peek(ctx, rule, "k"); p.Allowed {
		t.Fatal("peek should report limited")
	}
	if r := l.Hit(ctx, rule, "other"); !r.Allowed {
		t.Fatal("independent keys must not share a bucket")
	}
	mr.FastForward(61 * time.Second)
	if r := l.Hit(ctx, rule, "k"); !r.Allowed {
		t.Fatalf("limit should expire after the window: %+v", r)
	}
}

func TestPeekDoesNotCount(t *testing.T) {
	_, l := newRedis(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if p := l.Peek(ctx, rule, "k"); !p.Allowed {
			t.Fatal("peek must not consume the budget")
		}
	}
	l.Hit(ctx, rule, "k")
	l.Hit(ctx, rule, "k")
	if p := l.Peek(ctx, rule, "k"); !p.Allowed {
		t.Fatal("two of three used: next should be allowed")
	}
	l.Hit(ctx, rule, "k")
	if p := l.Peek(ctx, rule, "k"); p.Allowed || p.RetryAfter <= 0 {
		t.Fatalf("budget exhausted: %+v", p)
	}
	l.Reset(ctx, rule, "k")
	if p := l.Peek(ctx, rule, "k"); !p.Allowed {
		t.Fatal("reset should clear the counter")
	}
}

// When Redis goes away, limits keep being enforced by the in-process fallback.
func TestRedisOutageFallsBackToLocalLimits(t *testing.T) {
	mr, l := newRedis(t)
	ctx := context.Background()
	l.Hit(ctx, rule, "k")
	mr.Close()
	allowed := 0
	for i := 0; i < 10; i++ {
		r := l.Hit(ctx, rule, "k")
		if !r.Degraded {
			t.Fatal("expected degraded decision while Redis is down")
		}
		if r.Allowed {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("fallback allowed %d requests, want 3", allowed)
	}
	if !l.Degraded() {
		t.Fatal("limiter should report degraded")
	}
}

func TestLocalExpiry(t *testing.T) {
	l := New(nil, nil)
	now := time.Unix(1000, 0)
	l.local.now = func() time.Time { return now }
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		l.Hit(ctx, rule, "k")
	}
	if r := l.Hit(ctx, rule, "k"); r.Allowed {
		t.Fatal("should be limited")
	}
	now = now.Add(time.Minute)
	if r := l.Hit(ctx, rule, "k"); !r.Allowed {
		t.Fatal("should reset after window")
	}
}

func TestKeyHidesInput(t *testing.T) {
	k := Key("member@example.com")
	if len(k) != 32 || k == Key("other@example.com") {
		t.Fatalf("unexpected key %q", k)
	}
}
