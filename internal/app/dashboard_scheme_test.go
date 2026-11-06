package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/inventory"
)

// plainIngress is the installer's "upstream" setup: a load balancer
// ends TLS, so the Ingress has no TLS entry for the host.
func plainIngress(t *testing.T) *inventory.Model {
	t.Helper()
	model := inventory.NewModel(inventory.Options{})
	observeAddresses(t, model, ingressID("dash"),
		"http k.example.com / kwatch-dashboard dashboard")
	return model
}

// schemeToken signs in the requests visit sends: only signed-in
// requests count as seen.
const schemeToken = "scheme-test-token"

func newSchemeDashboard(opts dashboard.Options) *dashboard.Server {
	opts.Token = schemeToken
	return dashboard.NewServer(opts, clock.Func(func() time.Time {
		return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	}))
}

// visit sends one signed-in request for k.example.com that a proxy
// marked as https in X-Forwarded-Proto, or plain http when proto is "".
func visit(s *dashboard.Server, proto string) {
	r := httptest.NewRequest(http.MethodGet, "/api/incidents", nil)
	r.Host = "k.example.com"
	r.Header.Set("Authorization", "Bearer "+schemeToken)
	if proto != "" {
		r.Header.Set("X-Forwarded-Proto", proto)
	}
	s.Handler().ServeHTTP(httptest.NewRecorder(), r)
}

func TestDashboardAddressUsesHTTPSSeenThroughTrustedProxy(t *testing.T) {
	model := plainIngress(t)
	s := newSchemeDashboard(dashboard.Options{
		TrustedProxies: []string{"0.0.0.0/0"}})
	visit(s, "https")
	assert.Equal(t, "https://k.example.com",
		testDashboard.address(model, s))
}

func TestDashboardAddressIgnoresProtoHeaderWithoutTrust(t *testing.T) {
	model := plainIngress(t)
	s := newSchemeDashboard(dashboard.Options{})
	visit(s, "https")
	assert.Equal(t, "http://k.example.com",
		testDashboard.address(model, s))
}

func TestDashboardAddressHTTPSWhenSignInRefusesHTTP(t *testing.T) {
	model := plainIngress(t)
	s := newSchemeDashboard(dashboard.Options{TLSCertFile: "/tls/tls.crt"})
	assert.Equal(t, "https://k.example.com",
		testDashboard.address(model, s), "nothing seen yet")
	visit(s, "")
	assert.Equal(t, "https://k.example.com",
		testDashboard.address(model, s), "http seen but refused")
}

func TestDashboardAddressHTTPWhenSeenAndAllowed(t *testing.T) {
	model := plainIngress(t)
	s := newSchemeDashboard(dashboard.Options{})
	assert.Equal(t, "http://k.example.com",
		testDashboard.address(model, s), "nothing seen yet")
	visit(s, "")
	assert.Equal(t, "http://k.example.com",
		testDashboard.address(model, s))
}

func TestDashboardAddressKeepsIngressTLS(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	observeAddresses(t, model, ingressID("dash"),
		"https k.example.com / kwatch-dashboard dashboard")
	s := newSchemeDashboard(dashboard.Options{})
	visit(s, "")
	assert.Equal(t, "https://k.example.com",
		testDashboard.address(model, s))
}
