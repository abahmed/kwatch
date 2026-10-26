package dashboard

import (
	"crypto/subtle"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Auth validates bearer tokens for dashboard API access.
type Auth struct {
	token    atomic.Value // holds string
	failRate *rateLimiter
}

// NewAuth builds an auth checker with the given bearer token. An empty
// token means every request is rejected with 401.
func NewAuth(token string) *Auth {
	a := &Auth{}
	a.token.Store(token)
	a.failRate = newRateLimiter()
	return a
}

// SetToken replaces the bearer token (for rotation).
func (a *Auth) SetToken(token string) {
	a.token.Store(token)
}

// Middleware wraps an http.Handler with bearer token auth and per-IP
// rate limiting on failures.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.failRate.allowed(r.RemoteAddr) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"too many requests"}`))
			return
		}
		token, ok := bearerToken(r)
		if !ok || !a.valid(token) {
			a.failRate.record(r.RemoteAddr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// valid reports whether token matches the configured bearer token using
// constant-time comparison.
func (a *Auth) valid(token string) bool {
	expected, _ := a.token.Load().(string)
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

// bearerToken extracts the bearer token from the Authorization header.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", false
	}
	if len(auth) < len(prefix) || auth[:len(prefix)] != prefix {
		return "", false
	}
	return auth[len(prefix):], true
}

// rateLimiter tracks per-IP authentication failures to bound brute-force
// attempts. Fixed window per IP with a maximum count.
type rateLimiter struct {
	mu      sync.Mutex
	counts  map[string]int
	window  time.Time
	maxFail int
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		counts:  make(map[string]int),
		window:  time.Now(),
		maxFail: 10,
	}
}

func (r *rateLimiter) record(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rotate()
	r.counts[ip]++
}

func (r *rateLimiter) allowed(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rotate()
	return r.counts[ip] < r.maxFail
}

func (r *rateLimiter) rotate() {
	if time.Since(r.window) > time.Minute {
		r.window = time.Now()
		r.counts = make(map[string]int)
	}
}
