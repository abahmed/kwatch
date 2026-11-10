package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Some proxies append the client's port to X-Forwarded-For. The address
// part still names the client.
func TestForwardedClientAcceptsHopsWithAPort(t *testing.T) {
	auth := proxyAuth(t, "10.0.0.0/8")
	cases := []struct{ name, xff, want string }{
		{"ipv4 with port", "203.0.113.7:4711", "203.0.113.7"},
		{"ipv6 with port", "[2001:db8::7]:4711", "2001:db8::7"},
		{"ipv6 in brackets", "[2001:db8::7]", "2001:db8::7"},
		{"trusted hop with port is skipped",
			"203.0.113.7:4711, 10.2.2.2:80", "203.0.113.7"},
		{"port without address", ":4711", ""},
		{"name with port", "client.example:4711", ""},
		{"unclosed bracket", "[2001:db8::7", ""},
	}
	for _, c := range cases {
		got := auth.forwardedClient(proxyRequest("10.0.0.1", c.xff))
		if got != c.want {
			t.Errorf("%s: client = %q, want %q", c.name, got, c.want)
		}
	}
}

// One client's wrong passwords must not lock out the others behind a
// proxy that names clients as ip:port.
func TestRateLimitIsPerClientWhenTheProxyAddsPorts(t *testing.T) {
	forms := map[string][2]string{
		"ipv4": {"203.0.113.1:50000", "203.0.113.2:50001"},
		"ipv6": {"[2001:db8:1::1]:50000", "[2001:db8:2::1]:50001"},
	}
	for name, clients := range forms {
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
			fail(clients[0])
		}
		if fail(clients[0]) != http.StatusTooManyRequests {
			t.Errorf("%s: the guessing client was not limited", name)
		}
		if fail(clients[1]) != http.StatusUnauthorized {
			t.Errorf("%s: another client behind the proxy was limited",
				name)
		}
	}
}
