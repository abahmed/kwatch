package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
)

func proxyAuth(t *testing.T, list ...string) *Auth {
	t.Helper()
	auth := NewAuth("tok", "pw", len(list) > 0, clock.RealClock{})
	trusted, errs := parseProxies(list)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	auth.trusted = trusted
	return auth
}

func proxyRequest(peer string, xff ...string) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = peer + ":4000"
	for _, v := range xff {
		req.Header.Add("X-Forwarded-For", v)
	}
	return req
}

func TestForwardedClientWalksRightToLeftSkippingTrustedHops(t *testing.T) {
	auth := proxyAuth(t, "10.0.0.0/8", "192.168.1.9")
	cases := []struct {
		name string
		req  *http.Request
		want string
	}{
		{"one hop", proxyRequest("10.0.0.1", "203.0.113.7"),
			"203.0.113.7"},
		{"forged left entry is ignored",
			proxyRequest("10.0.0.1", "1.2.3.4, 203.0.113.7"),
			"203.0.113.7"},
		{"trusted hops are skipped",
			proxyRequest("10.0.0.1", "203.0.113.7, 10.2.2.2, 192.168.1.9"),
			"203.0.113.7"},
		{"repeated header lines join",
			proxyRequest("10.0.0.1", "1.1.1.1", "203.0.113.7, 10.9.9.9"),
			"203.0.113.7"},
		{"all hops trusted gives the leftmost",
			proxyRequest("10.0.0.1", "10.3.3.3, 10.2.2.2"), "10.3.3.3"},
		{"garbage falls back to the peer",
			proxyRequest("10.0.0.1", "203.0.113.7, junk"), ""},
		{"untrusted peer is ignored",
			proxyRequest("203.0.113.50", "198.51.100.1"), ""},
		{"loopback is not trusted by default",
			proxyRequest("127.0.0.1", "198.51.100.1"), ""},
	}
	for _, c := range cases {
		if got := auth.forwardedClient(c.req); got != c.want {
			t.Errorf("%s: client = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestForwardedProtoCountsOnlyFromTrustedPeers(t *testing.T) {
	auth := proxyAuth(t, "10.0.0.0/8")
	from := func(peer string) bool {
		req := proxyRequest(peer)
		req.Header.Set("X-Forwarded-Proto", "https")
		return auth.isHTTPS(req)
	}
	if !from("10.1.1.1") {
		t.Error("a trusted proxy's X-Forwarded-Proto was ignored")
	}
	if from("203.0.113.5") || from("127.0.0.1") {
		t.Error("X-Forwarded-Proto counted from an untrusted peer")
	}
}

func TestTrustedProxyOnLoopbackIsNotAPortForward(t *testing.T) {
	auth := proxyAuth(t, "127.0.0.1")
	if auth.loopback(proxyRequest("127.0.0.1")) {
		t.Error("a trusted loopback proxy counted as a port-forward")
	}
	local := proxyAuth(t)
	if !local.loopback(proxyRequest("127.0.0.1")) {
		t.Error("a port-forward was not seen as loopback")
	}
}

func TestParseProxiesRejectsGarbage(t *testing.T) {
	set, errs := parseProxies([]string{"10.0.0.0/8", "::1", "nope"})
	if len(set) != 2 || len(errs) != 1 {
		t.Fatalf("set = %v errs = %v", set, errs)
	}
}

func TestRateLimitIsPerRealClientBehindTrustedProxy(t *testing.T) {
	auth := proxyAuth(t, "10.0.0.0/8")
	h := auth.Middleware(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {}))
	fail := func(client string) int {
		req := proxyRequest("10.0.0.1", client)
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("Authorization", "Bearer wrong")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}
	for range maxFailures {
		fail("203.0.113.1")
	}
	if fail("203.0.113.1") != http.StatusTooManyRequests {
		t.Error("the guessing client was not limited")
	}
	if fail("203.0.113.2") != http.StatusUnauthorized {
		t.Error("another client behind the proxy was limited")
	}
}
