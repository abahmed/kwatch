package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
)

func TestAuthValidToken(t *testing.T) {
	auth := NewAuth("secret-token", "", false, clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMissingToken(t *testing.T) {
	auth := NewAuth("secret-token", "", false, clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthInvalidToken(t *testing.T) {
	auth := NewAuth("secret-token", "", false, clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRateLimit(t *testing.T) {
	auth := NewAuth("secret-token", "", false, clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/api/incidents", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("attempt %d: status = %d", i, w.Code)
		}
	}
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
	// The limiter counts failures only: the right token is never held
	// back by it.
	req = httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("right token past the limit: status = %d, want 200",
			w.Code)
	}
}

func TestAuthEmptyTokenRejectsAll(t *testing.T) {
	auth := NewAuth("", "", false, clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer anything")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRateLimitFollowsTheHostAcrossConnections(t *testing.T) {
	handler := NewAuth("secret-token", "", false, clock.RealClock{}).Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
	for port := range 10 {
		req := httptest.NewRequest("GET", "/api/incidents", nil)
		req.RemoteAddr = "10.1.2.3:" + strconv.Itoa(40000+port)
		req.Header.Set("Authorization", "Bearer wrong-token")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	limited := httptest.NewRequest("GET", "/api/incidents", nil)
	limited.RemoteAddr = "10.1.2.3:50000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, limited)
	other := httptest.NewRequest("GET", "/api/incidents", nil)
	other.RemoteAddr = "10.9.9.9:40000"
	other.Header.Set("Authorization", "Bearer secret-token")
	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, other)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("same host, new port: status = %d, want 429", w.Code)
	}
	if ok.Code != http.StatusOK {
		t.Errorf("other host: status = %d, want 200", ok.Code)
	}
}

func okHandler(auth *Auth) http.Handler {
	return auth.Middleware(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {}))
}

// The Bearer scheme is case-insensitive, as HTTP schemes are.
func TestAuthBearerSchemeIsCaseInsensitive(t *testing.T) {
	handler := okHandler(NewAuth("secret-token", "", false,
		clock.RealClock{}))
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "bearer secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("lower-case scheme: status = %d, want 200", w.Code)
	}
}

// Another scheme is no bearer token: it neither hides a valid session
// cookie nor counts as a guess.
func TestAuthOtherSchemeFallsThroughToTheCookie(t *testing.T) {
	auth := NewAuth("secret-token", "pw", false, clock.RealClock{})
	handler := okHandler(auth)
	session, _, err := auth.sessions.issue()
	if err != nil {
		t.Fatal(err)
	}
	req := cookieRequest(session)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Basic header with a valid cookie: status = %d", w.Code)
	}
	for range 3 * maxFailures {
		req := httptest.NewRequest("GET", "/api/incidents", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	if !auth.allowed(cookieRequest("")) {
		t.Error("a non-bearer header counted as a guess")
	}
}

// A wrong bearer token beside a valid session cookie lets the cookie
// in and does not count as a guess.
func TestAuthWrongBearerWithValidCookieIsNotAGuess(t *testing.T) {
	auth := NewAuth("secret-token", "pw", false, clock.RealClock{})
	handler := okHandler(auth)
	session, _, err := auth.sessions.issue()
	if err != nil {
		t.Fatal(err)
	}
	for range 3 * maxFailures {
		req := cookieRequest(session)
		req.Header.Set("Authorization", "Bearer wrong")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	}
	if !auth.allowed(cookieRequest("")) {
		t.Error("a wrong bearer beside a valid cookie counted")
	}
}

func TestAuthValidTokenIgnoresLength(t *testing.T) {
	auth := NewAuth("secret-token", "", false, clock.RealClock{})
	if !auth.validToken("secret-token") || auth.validToken("secret") ||
		auth.validToken("secret-token-and-more") {
		t.Error("validToken accepts or refuses the wrong tokens")
	}
}

// A trusted proxy's X-Forwarded-For that is not an address cannot pick
// a fresh bucket: the peer is limited instead.
func TestAuthLimiterIgnoresForwardedGarbage(t *testing.T) {
	auth := NewAuth("secret-token", "", false, clock.RealClock{})
	auth.trusted, _ = parseProxies([]string{"10.0.0.0/8"})
	for i := range 2 * maxFailures {
		req := httptest.NewRequest("GET", "/api/incidents", nil)
		req.RemoteAddr = "10.1.2.3:4000"
		req.Header.Set("X-Forwarded-For", "junk-"+strconv.Itoa(i))
		req.Header.Set("Authorization", "Bearer wrong")
		okHandler(auth).ServeHTTP(httptest.NewRecorder(), req)
	}
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.RemoteAddr = "10.1.2.3:4000"
	req.Header.Set("X-Forwarded-For", "junk-new")
	if auth.allowed(req) {
		t.Error("garbage forwarded values escaped the peer's limit")
	}
	if len(auth.clientRate.counts) != 0 {
		t.Error("garbage forwarded values were stored as clients")
	}
}

func TestAuthForwardedClientIsCanonical(t *testing.T) {
	auth := NewAuth("secret-token", "", false, clock.RealClock{})
	auth.trusted, _ = parseProxies([]string{"10.0.0.0/8"})
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.1.2.3:4000"
	req.Header.Set("X-Forwarded-For", "evil, 2001:0DB8:0:0:0:0:0:1")
	if got := auth.forwardedClient(req); got != "2001:db8::1" {
		t.Errorf("forwardedClient = %q, want 2001:db8::1", got)
	}
}

// Past maxLimiterKeys clients the oldest is forgotten, so the map stays
// bounded and a client past the bound keeps its own count.
func TestRateLimiterCapsItsKeys(t *testing.T) {
	l := newRateLimiter(clock.RealClock{}, maxFailures)
	for i := range maxLimiterKeys + 500 {
		l.record("client-" + strconv.Itoa(i))
	}
	if n := len(l.counts); n != maxLimiterKeys {
		t.Errorf("keys = %d, want %d", n, maxLimiterKeys)
	}
	if !l.allowed("client-" + strconv.Itoa(maxLimiterKeys+1)) {
		t.Error("a client past the bound shares a full bucket")
	}
}
