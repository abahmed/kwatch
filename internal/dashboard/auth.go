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
	// client a trusted proxy names. proxyRate only counts the failures
	// that came through each proxy, to log when there are many: it
	// never refuses, so guesses from some clients cannot lock out
	// every other client behind the same proxy.
	failRate   *rateLimiter
	clientRate *rateLimiter
	proxyRate  *rateLimiter
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
		proxyRate:  newRateLimiter(now, maxProxyFailures),
	}
}

// Middleware lets a request through with a valid bearer token or
// session cookie. A bearer token can be guessed: each one reserves an
// attempt from the client's failure limit (a right one gives it back),
// and past the limit even a right one is refused, so a guess that
// happens to be right cannot get in. A session cookie cannot be guessed
// (its ID is random and only a sign-in makes one): a valid one is let
// through even while the client is limited, so guesses from a shared
// address do not lock out people already signed in, and an invalid one
// does not count, because after a restart every open tab presents one
// on each poll. In shared mode a bearer token must come over https,
// except from loopback (a port-forward).
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, guess := a.check(r)
		if ok && !guess {
			next.ServeHTTP(w, r)
			return
		}
		if guess && a.shared && !a.isHTTPS(r) && !loopback(r) {
			writeError(w, http.StatusForbidden,
				"the token must be sent over https")
			return
		}
		if !guess {
			if !a.allowed(r) {
				writeError(w, http.StatusTooManyRequests,
					"too many requests")
				return
			}
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		attempt, reserved := a.reserve(r)
		if !reserved {
			writeError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		if !ok {
			a.noteProxyFailure(r)
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		attempt.forgive()
		next.ServeHTTP(w, r)
	})
}

// check reports whether r carries a valid credential and whether a
// failed one counts as a guess: a bearer token does, a session cookie
// (random, so never guessed) and no credential at all do not. A valid
// session cookie wins over a bearer token that is wrong, and that is
// not a guess.
func (a *Auth) check(r *http.Request) (ok, guess bool) {
	token, has := bearerToken(r)
	if has && a.validToken(token) {
		return true, true
	}
	if a.validCookie(r) {
		return true, false
	}
	return false, has
}

// validCookie reports whether r carries a valid session cookie.
func (a *Auth) validCookie(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" || a.password == "" {
		return false
	}
	return a.sessions.valid(c.Value)
}

// validToken compares token with the configured bearer token in
// constant time. Both are hashed first, so the length of the right
// token does not leak.
func (a *Auth) validToken(token string) bool {
	if a.token == "" {
		return false
	}
	got := sha256.Sum256([]byte(token))
	want := sha256.Sum256([]byte(a.token))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
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
// The scheme is matched without regard to case; a header with another
// scheme (Basic, say) is not a bearer token and is ignored.
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if len(auth) < len(prefix) ||
		!strings.EqualFold(auth[:len(prefix)], prefix) {
		return "", false
	}
	return auth[len(prefix):], true
}

// rateLimiter tracks per-client authentication failures to bound
// brute-force attempts. Fixed window per client with a maximum count.
type rateLimiter struct {
	mu     sync.Mutex
	counts map[string]int
	window time.Time
	// gen counts the windows, so an attempt reserved in one window is
	// never given back in the next.
	gen     uint64
	now     func() time.Time
	maxFail int
}

// maxLimiterKeys bounds the clients one window remembers. Past it new
// clients share the overflowKey bucket, so memory stays bounded.
const (
	maxLimiterKeys = 10000
	overflowKey    = "\x00overflow"
)

// Failures allowed per minute per client. maxProxyFailures is how many
// failures through one trusted proxy in a minute are logged as a
// warning; it never refuses a request.
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

// bucket is the key ip is counted under: itself, or the shared
// overflowKey when the window already holds maxLimiterKeys others.
// The caller holds the lock.
func (r *rateLimiter) bucket(ip string) string {
	if _, ok := r.counts[ip]; ok || len(r.counts) < maxLimiterKeys {
		return ip
	}
	return overflowKey
}

// record counts a failure and returns the count in this window.
func (r *rateLimiter) record(ip string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rotate()
	ip = r.bucket(ip)
	r.counts[ip]++
	return r.counts[ip]
}

func (r *rateLimiter) allowed(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rotate()
	return r.counts[r.bucket(ip)] < r.maxFail
}

// attempt is a reserved try: it counts as a failure unless forgiven.
type attempt struct {
	limiter *rateLimiter
	key     string
	gen     uint64
}

// reserve checks the limit and counts one attempt under one lock, so
// requests that arrive together cannot all pass the check before any
// of them is counted. false when the limit is reached.
func (r *rateLimiter) reserve(key string) (attempt, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rotate()
	key = r.bucket(key)
	if r.counts[key] >= r.maxFail {
		return attempt{}, false
	}
	r.counts[key]++
	return attempt{limiter: r, key: key, gen: r.gen}, true
}

// forgive gives a reserved attempt back: it was not a failure. An
// attempt from a window that has ended is already forgotten.
func (t attempt) forgive() {
	r := t.limiter
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gen == t.gen && r.counts[t.key] > 0 {
		r.counts[t.key]--
	}
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
		r.gen++
	}
}
