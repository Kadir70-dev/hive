// Package ratelimit implements fixed-window limits backed by Redis, with an
// in-process fallback so a Redis outage never removes protection.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Rule is a limit of Limit events per Window.
type Rule struct {
	Name   string
	Limit  int
	Window time.Duration
}

// Result describes a limiter decision.
type Result struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
	Degraded   bool // true when the in-process fallback decided
}

// Limiter checks and counts events.
type Limiter struct {
	rdb      redis.UniversalClient
	local    *local
	timeout  time.Duration
	log      *slog.Logger
	mu       sync.Mutex
	degraded bool
}

// incrScript atomically increments a counter and sets its expiry on first use.
var incrScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
local ttl = redis.call('PTTL', KEYS[1])
if ttl < 0 then redis.call('PEXPIRE', KEYS[1], ARGV[1]); ttl = tonumber(ARGV[1]) end
return {n, ttl}
`)

// New returns a limiter. rdb may be nil, in which case only the in-process
// fallback is used (suitable for tests and single-instance development).
func New(rdb redis.UniversalClient, log *slog.Logger) *Limiter {
	if log == nil {
		log = slog.Default()
	}
	return &Limiter{rdb: rdb, local: newLocal(), timeout: 250 * time.Millisecond, log: log}
}

// Key hashes identifying material (emails, IPs) so Redis never stores it raw.
func Key(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func (l *Limiter) redisKey(rule Rule, key string) string { return "hive:rl:" + rule.Name + ":" + key }

// Hit counts one event and reports whether it is within the rule.
func (l *Limiter) Hit(ctx context.Context, rule Rule, key string) Result {
	if l.rdb != nil {
		c, cancel := context.WithTimeout(ctx, l.timeout)
		defer cancel()
		vals, err := incrScript.Run(c, l.rdb, []string{l.redisKey(rule, key)}, rule.Window.Milliseconds()).Int64Slice()
		if err == nil && len(vals) == 2 {
			l.setDegraded(false)
			n, ttl := int(vals[0]), time.Duration(vals[1])*time.Millisecond
			return decide(rule, n, ttl, false)
		}
		l.setDegraded(true)
	}
	n, ttl := l.local.incr(rule.Name+":"+key, rule.Window)
	return decide(rule, n, ttl, true)
}

// Peek reports whether one more event would be allowed, without counting it.
func (l *Limiter) Peek(ctx context.Context, rule Rule, key string) Result {
	if l.rdb != nil {
		c, cancel := context.WithTimeout(ctx, l.timeout)
		defer cancel()
		k := l.redisKey(rule, key)
		n, err := l.rdb.Get(c, k).Int()
		if err == nil || err == redis.Nil {
			ttl, terr := l.rdb.PTTL(c, k).Result()
			if terr == nil {
				l.setDegraded(false)
				return decide(rule, n+1, ttl, false)
			}
		}
		l.setDegraded(true)
	}
	n, ttl := l.local.get(rule.Name + ":" + key)
	return decide(rule, n+1, ttl, true)
}

// Reset clears a counter (used after a successful login).
func (l *Limiter) Reset(ctx context.Context, rule Rule, key string) {
	if l.rdb != nil {
		c, cancel := context.WithTimeout(ctx, l.timeout)
		defer cancel()
		_ = l.rdb.Del(c, l.redisKey(rule, key)).Err()
	}
	l.local.del(rule.Name + ":" + key)
}

// Degraded reports whether the last Redis interaction failed.
func (l *Limiter) Degraded() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.degraded
}

func (l *Limiter) setDegraded(v bool) {
	l.mu.Lock()
	changed := l.degraded != v
	l.degraded = v
	l.mu.Unlock()
	if changed && v {
		l.log.Warn("rate limiter: Redis unavailable, using in-process fallback limits")
	} else if changed {
		l.log.Info("rate limiter: Redis available again")
	}
}

func decide(rule Rule, n int, ttl time.Duration, degraded bool) Result {
	if ttl <= 0 {
		ttl = rule.Window
	}
	r := Result{Allowed: n <= rule.Limit, Remaining: rule.Limit - n, Degraded: degraded}
	if r.Remaining < 0 {
		r.Remaining = 0
	}
	if !r.Allowed {
		r.RetryAfter = ttl
	}
	return r
}

// local is a bounded in-process fixed-window counter.
type local struct {
	mu      sync.Mutex
	entries map[string]*entry
	now     func() time.Time
}

type entry struct {
	n       int
	expires time.Time
}

const localMaxEntries = 100000

func newLocal() *local { return &local{entries: map[string]*entry{}, now: time.Now} }

func (s *local) incr(key string, window time.Duration) (int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	e, ok := s.entries[key]
	if !ok || !now.Before(e.expires) {
		if len(s.entries) >= localMaxEntries {
			s.sweep(now)
		}
		e = &entry{expires: now.Add(window)}
		s.entries[key] = e
	}
	e.n++
	return e.n, e.expires.Sub(now)
}

func (s *local) get(key string) (int, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	e, ok := s.entries[key]
	if !ok || !now.Before(e.expires) {
		return 0, 0
	}
	return e.n, e.expires.Sub(now)
}

func (s *local) del(key string) {
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
}

// sweep drops expired entries; if still full it resets, preferring memory
// safety over perfect counting under a key-flooding attack.
func (s *local) sweep(now time.Time) {
	for k, e := range s.entries {
		if !now.Before(e.expires) {
			delete(s.entries, k)
		}
	}
	if len(s.entries) >= localMaxEntries {
		s.entries = map[string]*entry{}
	}
}
