package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// originReq is a POST /login for host (the Host header the server
// sees) from peer, with the browser's Origin and the proxy's
// X-Forwarded-Host.
func originReq(peer, origin string, fwd ...string) *http.Request {
	req := loginRequest(testPassword)
	req.Host = "kwatch.svc:8080"
	req.RemoteAddr = peer + ":4000"
	req.Header.Set("Origin", origin)
	for _, f := range fwd {
		req.Header.Add("X-Forwarded-Host", f)
	}
	return req
}

// Behind a trusted proxy that rewrites Host, the Origin matches the
// proxy's X-Forwarded-Host. Any other peer's header counts for nothing.
func TestSameOriginUsesForwardedHostOnlyFromTrustedPeers(t *testing.T) {
	s := newSessionServer(true, testPassword)
	for name, c := range map[string]struct {
		req  *http.Request
		code int
	}{
		"trusted peer, matching host": {originReq("10.1.2.3",
			"https://dash.example.com", "dash.example.com"), 204},
		"trusted peer, last value wins": {originReq("10.1.2.3",
			"https://dash.example.com", "evil.example, dash.example.com"),
			204},
		"trusted peer, other host": {originReq("10.1.2.3",
			"https://evil.example", "dash.example.com"), 403},
		"trusted peer, odd value": {originReq("10.1.2.3",
			"https://evil.example/x", "evil.example/x"), 403},
		"untrusted peer, matching host": {originReq("203.0.113.9",
			"https://dash.example.com", "dash.example.com"), 403},
	} {
		c.req.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, c.req)
		if w.Code != c.code {
			t.Errorf("%s: %d, want %d", name, w.Code, c.code)
		}
	}
}

func TestPushAcceptsForwardedHostFromTrustedPeer(t *testing.T) {
	s := newSessionServer(true, testPassword)
	call := func(peer string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/push/status",
			nil)
		req.Host = "kwatch.svc:8080"
		req.RemoteAddr = peer + ":4000"
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://dash.example.com")
		req.Header.Set("X-Forwarded-Host", "dash.example.com")
		req.Header.Set("X-Forwarded-Proto", "https")
		return s.do(req).Code
	}
	if c := call("203.0.113.9"); c != 403 {
		t.Errorf("untrusted: %d, want 403", c)
	}
	if c := call("10.1.2.3"); c == 403 {
		t.Errorf("trusted: %d, want past the origin check", c)
	}
}
