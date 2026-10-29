package dashboard

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// Auth checks access to /api/*: the bearer token or a session cookie
// from a password sign-in. Failures share one per-client rate limit.
type Auth struct {
	token    string
	password string
	shared   bool
	// trustProxy honours X-Forwarded-Proto and X-Forwarded-For from a
	// proxy that terminates TLS. Set by NewServer from Options.
	trustProxy bool
	sessions   *sessions
	// failRate limits failures per connecting peer; clientRate per
	// client a trusted proxy names.
	failRate   *rateLimiter
	clientRate *rateLimiter
}

// NewAuth builds the access checker. An empty token refuses bearer
// access and an empty password refuses sign-in. shared marks the cookie
// Secure and refuses sign-in over plain HTTP. now is the clock of
// sessions and of the rate-limit window; nil is rejected.
func NewAuth(token, password string, shared bool, now clock.Clock) *Auth {
	return &Auth{
		token:      token,
		password:   normalizePassword(password),
		shared:     shared,
		sessions:   newSessions(normalizePassword(password), token, now),
		failRate:   newRateLimiter(now, maxFailures),
		clientRate: newRateLimiter(now, maxFailures),
	}
}

// Middleware lets a request through with a valid bearer token or
// session cookie. A bearer token can be guessed: a wrong one counts
// against the client's failure limit, and past the limit even a right
// one is refused, so a guess that happens to be right cannot get in. A
// session cookie cannot be guessed (its ID is random and only a sign-in
// makes one): a valid one is let through even while the address is
// limited, so guesses from a shared address do not lock out people
// already signed in, and an invalid one does not count, because after a
// restart every open tab presents one on each poll.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, guess := a.check(r)
		if ok && !guess {
			next.ServeHTTP(w, r)
			return
		}
		if !a.allowed(r) {
			writeError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		if !ok {
			if guess {
				a.recordFailure(r)
			}
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// check reports whether r carries a valid credential and whether a
// failed one counts as a guess: a bearer token does, a session cookie
// (random, so never guessed) and no credential at all do not.
func (a *Auth) check(r *http.Request) (ok, guess bool) {
	if token, has := bearerToken(r); has {
		return a.validToken(token), true
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if a.password == "" {
			return false, false
		}
		return a.sessions.valid(c.Value), false
	}
	return false, false
}

// validToken compares token with the configured bearer token in
// constant time.
func (a *Auth) validToken(token string) bool {
	if a.token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.token)) == 1
}

// validPassword compares a typed password with the configured one in
// constant time. Both are normalized first, so the installer's grouped
// form is accepted with or without dashes and in any case.
func (a *Auth) validPassword(typed string) bool {
	if a.password == "" {
		return false
	}
	got := sha256.Sum256([]byte(normalizePassword(typed)))
	want := sha256.Sum256([]byte(a.password))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// normalizePassword drops dashes and spaces and upper-cases the rest.
func normalizePassword(p string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, p))
}

// bearerToken extracts the bearer token from the Authorization header.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", false
	}
	if len(auth) < len(prefix) || auth[:len(prefix)] != prefix {
		return "", true
	}
	return auth[len(prefix):], true
}

// rateLimiter tracks per-client authentication failures to bound
// brute-force attempts. Fixed window per client with a maximum count.
type rateLimiter struct {
	mu      sync.Mutex
	counts  map[string]int
	window  time.Time
	now     func() time.Time
	maxFail int
}

// Failures allowed per minute: per client, and per connecting peer when
// a trusted proxy carries many clients.
const (
	maxFailures      = 10
	maxProxyFailures = 100
)

func newRateLimiter(now clock.Clock, maxFail int) *rateLimiter {
	return &rateLimiter{
		counts:  make(map[string]int),
		window:  now.Now(),
		now:     now.Now,
		maxFail: maxFail,
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

// retryAfter is how long until the current window ends.
func (r *rateLimiter) retryAfter() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return max(time.Minute-r.now().Sub(r.window), time.Second)
}

func (r *rateLimiter) rotate() {
	if r.now().Sub(r.window) > time.Minute {
		r.window = r.now()
		r.counts = make(map[string]int)
	}
}
