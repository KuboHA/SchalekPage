package web

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// loginRateLimit and loginRateWindow bound how many /login attempts a single
// client IP may make before being told to slow down. This is a coarse,
// in-memory brute-force guard, not a substitute for EduPage's own account
// protections — it exists because handleLogin forwards credentials straight
// through to EduPage on every request, with nothing else standing between
// an attacker and repeated password guesses.
const (
	loginRateLimit  = 5
	loginRateWindow = 5 * time.Minute
)

// rateLimiter is a fixed-window request counter keyed by an arbitrary string
// (here, client IP). It follows the same lock-a-map-of-small-structs shape
// as Store, deliberately kept dependency-free.
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time

	buckets map[string]*rateBucket
}

type rateBucket struct {
	count       int
	windowStart time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:   limit,
		window:  window,
		now:     time.Now,
		buckets: make(map[string]*rateBucket),
	}
}

// Allow reports whether another attempt for key is permitted right now, and
// counts it against the window if so.
func (rl *rateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	b, ok := rl.buckets[key]
	if !ok || now.Sub(b.windowStart) > rl.window {
		b = &rateBucket{count: 0, windowStart: now}
		rl.buckets[key] = b
	}
	if b.count >= rl.limit {
		return false
	}
	b.count++
	return true
}

// reap drops buckets whose window has fully elapsed, so a flood of one-off
// IPs doesn't grow the map forever.
func (rl *rateLimiter) reap() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	for key, b := range rl.buckets {
		if now.Sub(b.windowStart) > rl.window {
			delete(rl.buckets, key)
		}
	}
}

// StartReaper runs a background goroutine that periodically evicts expired
// buckets until stop is closed.
func (rl *rateLimiter) StartReaper(stop <-chan struct{}, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				rl.reap()
			}
		}
	}()
}

// clientIP extracts the request's IP for rate-limiting purposes. It reads
// only RemoteAddr, not X-Forwarded-For: that header is attacker-controlled
// unless a trusted proxy overwrites it, which would make the limiter trivial
// to bypass by spoofing it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
