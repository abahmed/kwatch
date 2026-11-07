package dashboard

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
)

func newSchemeServer(trustProxy bool) *Server {
	var proxies []string
	if trustProxy {
		proxies = []string{"0.0.0.0/0", "::/0"}
	}
	return NewServer(Options{Token: testToken, TrustedProxies: proxies},
		clock.Func(func() time.Time { return testEpoch }))
}

// serveHost sends a signed-in GET /api/session for host, with
// X-Forwarded-Proto when proto is not empty, from a client that is not
// loopback.
func serveHost(s *Server, host, proto string) {
	serveHostAs(s, host, proto, testToken)
}

// serveHostAs is serveHost with token as the bearer token; "" sends
// none.
func serveHostAs(s *Server, host, proto, token string) {
	r := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	r.Host = host
	if proto != "" {
		r.Header.Set("X-Forwarded-Proto", proto)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	s.Handler().ServeHTTP(httptest.NewRecorder(), r)
}

func TestSchemeForTrustsForwardedProtoBehindProxy(t *testing.T) {
	s := newSchemeServer(true)
	_, ok := s.SchemeFor("k.example.com")
	assert.False(t, ok, "nothing seen yet")
	serveHost(s, "K.example.com:443", "https")
	scheme, ok := s.SchemeFor("k.example.com")
	assert.True(t, ok)
	assert.Equal(t, "https", scheme)
	serveHost(s, "k.example.com", "")
	scheme, _ = s.SchemeFor("K.Example.com")
	assert.Equal(t, "https", scheme, "a plain request does not downgrade")
	serveHost(s, "plain.example.com", "")
	serveHost(s, "plain.example.com", "https")
	scheme, _ = s.SchemeFor("plain.example.com")
	assert.Equal(t, "https", scheme, "https replaces http")
}

// Requests that are not signed in record nothing, so made-up Host
// headers can neither add hosts nor push the real one out.
func TestSchemeForIgnoresRequestsThatAreNotSignedIn(t *testing.T) {
	s := newSchemeServer(true)
	serveHost(s, "k.example.com", "https")
	for i := range maxSchemeHosts * 2 {
		host := fmt.Sprintf("junk%d.example.com", i)
		serveHostAs(s, host, "http", "")
		serveHostAs(s, host, "http", "wrong-token")
	}
	_, ok := s.SchemeFor("junk0.example.com")
	assert.False(t, ok, "unauthenticated host recorded")
	scheme, ok := s.SchemeFor("k.example.com")
	assert.True(t, ok, "the real host was pushed out")
	assert.Equal(t, "https", scheme)
}

func TestSchemeForIgnoresForwardedProtoWithoutTrust(t *testing.T) {
	s := newSchemeServer(false)
	serveHost(s, "k.example.com", "https")
	scheme, ok := s.SchemeFor("k.example.com")
	assert.True(t, ok)
	assert.Equal(t, "http", scheme)
}

func TestSchemeLogKeepsAtMostSixteenHosts(t *testing.T) {
	s := newSchemeServer(true)
	for i := range maxSchemeHosts + 4 {
		serveHost(s, fmt.Sprintf("h%d.example.com", i), "https")
	}
	assert.Len(t, s.schemes.byHost, maxSchemeHosts)
	assert.Len(t, s.schemes.order, maxSchemeHosts)
	_, ok := s.SchemeFor("h0.example.com")
	assert.False(t, ok, "the oldest host is forgotten")
	_, ok = s.SchemeFor(fmt.Sprintf("h%d.example.com",
		maxSchemeHosts+3))
	assert.True(t, ok)
}

func TestRequiresHTTPSFollowsSharedMode(t *testing.T) {
	assert.False(t, newSchemeServer(false).RequiresHTTPS())
	shared := NewServer(Options{TrustedProxies: []string{"10.0.0.0/8"}},
		clock.Func(func() time.Time { return testEpoch }))
	assert.True(t, shared.RequiresHTTPS())
}
