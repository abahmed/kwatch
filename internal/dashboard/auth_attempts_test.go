package dashboard

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// slowBody signals when a request starts reading it, then waits until
// released, like a client that sends its body slowly.
type slowBody struct {
	started chan<- struct{}
	release <-chan struct{}
	body    io.Reader
	waited  bool
}

func (b *slowBody) Read(p []byte) (int, error) {
	if !b.waited {
		b.waited = true
		b.started <- struct{}{}
		<-b.release
	}
	return b.body.Read(p)
}

// Many sign-ins sent at once with slow bodies must not all pass the
// limit before any of them is counted: each reserves its attempt
// before the body is read.
func TestAuthLoginReservesAttemptsBeforeReadingTheBody(t *testing.T) {
	s := newSessionServer(false, testPassword)
	handler := s.Handler()
	const n = 3 * maxFailures
	started := make(chan struct{}, n)
	release := make(chan struct{})
	codes := make(chan int, n)
	for range n {
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/login",
				&slowBody{started: started, release: release,
					body: strings.NewReader(`{"password":"wrong"}`)})
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			codes <- w.Code
		}()
	}
	reading, refused := 0, 0
	timeout := time.After(5 * time.Second)
	for reading+refused < n {
		select {
		case <-started:
			reading++
		case code := <-codes:
			if code != http.StatusTooManyRequests {
				t.Fatalf("early answer %d, want 429", code)
			}
			refused++
		case <-timeout:
			t.Fatalf("stuck: %d reading, %d refused", reading, refused)
		}
	}
	close(release)
	for range reading {
		if code := <-codes; code != http.StatusUnauthorized {
			t.Errorf("wrong password: %d, want 401", code)
		}
	}
	if reading != maxFailures {
		t.Errorf("%d attempts read their body, want %d", reading,
			maxFailures)
	}
}

// A right password and a bad body give their attempt back.
func TestAuthLoginForgivesSuccessAndBadRequests(t *testing.T) {
	s := newSessionServer(false, testPassword)
	for range 2 * maxFailures {
		s.signIn(t, testPassword)
		bad := httptest.NewRequest(http.MethodPost, "/login",
			strings.NewReader("{"))
		bad.Header.Set("Content-Type", "application/json")
		if code := s.do(bad).Code; code != http.StatusBadRequest {
			t.Fatalf("bad body: %d, want 400", code)
		}
	}
	if !s.auth.allowed(loginRequest("")) {
		t.Error("successful sign-ins used up the failure limit")
	}
}

// In shared mode a bearer token sent over plain http is refused before
// it is checked; over https, and from loopback (a port-forward), it
// works.
func TestAuthSharedModeRefusesTheTokenOverHTTP(t *testing.T) {
	s := newSessionServer(true, testPassword)
	send := func(remote, proto string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/incidents", nil)
		req.RemoteAddr = remote
		if proto != "" {
			req.Header.Set("X-Forwarded-Proto", proto)
		}
		req.Header.Set("Authorization", "Bearer "+testToken)
		return s.do(req)
	}

	plain := send("10.0.0.5:1234", "")
	if plain.Code != http.StatusForbidden ||
		!strings.Contains(plain.Body.String(), "https") {
		t.Errorf("plain http: %d %s", plain.Code, plain.Body.String())
	}
	if w := send("10.0.0.5:1234", "https"); w.Code != http.StatusOK {
		t.Errorf("https: %d, want 200", w.Code)
	}
	if w := send("127.0.0.1:1234", ""); w.Code != http.StatusOK {
		t.Errorf("port-forward: %d, want 200", w.Code)
	}
	local := newSessionServer(false, testPassword)
	req := httptest.NewRequest(http.MethodGet, "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	if w := local.do(req); w.Code != http.StatusOK {
		t.Errorf("local mode over http: %d, want 200", w.Code)
	}
}

// Shared mode tells browsers to keep using https, but only on an https
// response: browsers ignore it over http, and local mode never sends
// it.
func TestAuthSharedModeSendsHSTSOverHTTPS(t *testing.T) {
	hsts := func(s *sessionServer, req *http.Request) string {
		return s.do(req).Header().Get("Strict-Transport-Security")
	}
	shared := newSessionServer(true, testPassword)
	viaIngress := httptest.NewRequest(http.MethodGet, "/", nil)
	viaIngress.RemoteAddr = "10.0.0.5:1234"
	viaIngress.Header.Set("X-Forwarded-Proto", "https")
	plain := httptest.NewRequest(http.MethodGet, "/", nil)
	plain.RemoteAddr = "10.0.0.5:1234"
	local := newSessionServer(false, testPassword)
	direct := httptest.NewRequest(http.MethodGet, "/", nil)
	direct.TLS = &tls.ConnectionState{}

	if got := hsts(shared, viaIngress); got != hstsValue {
		t.Errorf("shared over https: %q, want %q", got, hstsValue)
	}
	if got := hsts(shared, plain); got != "" {
		t.Errorf("shared over http: %q, want none", got)
	}
	if got := hsts(local, direct); got != "" {
		t.Errorf("local mode: %q, want none", got)
	}
}
