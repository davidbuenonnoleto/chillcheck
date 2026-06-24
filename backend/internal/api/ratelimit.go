package api

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// rateLimiter is a small per-client fixed-window limiter used to throttle the
// unauthenticated auth endpoints (login, register, forgot, reset, invite
// accept) against brute-force and credential-stuffing. In-memory and per
// process — fine for the single/low-replica App Runner deployment; a
// distributed limiter (Redis) would be the move if that assumption changes.
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string]*window
	limit  int
	window time.Duration
	now    func() time.Time // injectable for tests
}

type window struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(limit int, per time.Duration) *rateLimiter {
	return &rateLimiter{
		hits:   make(map[string]*window),
		limit:  limit,
		window: per,
		now:    time.Now,
	}
}

// allow reports whether a request from key is permitted, and how long until the
// window resets when it is not.
func (rl *rateLimiter) allow(key string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	w, ok := rl.hits[key]
	if !ok || now.After(w.resetAt) {
		rl.hits[key] = &window{count: 1, resetAt: now.Add(rl.window)}
		rl.purge(now)
		return true, 0
	}
	if w.count >= rl.limit {
		return false, w.resetAt.Sub(now)
	}
	w.count++
	return true, 0
}

// purge drops expired windows so the map can't grow without bound. Called only
// when a fresh window is created (cheap, amortized), under the held lock.
func (rl *rateLimiter) purge(now time.Time) {
	if len(rl.hits) < 1024 {
		return
	}
	for k, w := range rl.hits {
		if now.After(w.resetAt) {
			delete(rl.hits, k)
		}
	}
}

// middleware throttles by client IP and returns 429 with Retry-After when over
// the limit.
func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retry := rl.allow(clientIP(r))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			writeErr(w, http.StatusTooManyRequests, "too many attempts — please wait a moment and try again")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP resolves the caller's address, honoring the leftmost X-Forwarded-For
// entry set by App Runner / a load balancer, and falling back to RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
