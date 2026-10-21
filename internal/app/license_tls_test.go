package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// app.insecureSkipTLSVerify belongs to the provider client. The license
// key must travel over a client that always verifies TLS.
func TestLicenseManagerUsesThePlatformHTTPClient(t *testing.T) {
	deps := activeDeps(t)
	provider := &http.Transport{}
	platform := &http.Transport{}
	deps.clients.HTTP = &http.Client{Transport: provider}
	deps.clients.PlatformHTTP = &http.Client{Transport: platform}
	l := &licenseSession{
		deps: deps, now: time.Now,
		tracker: &renewTracker{now: time.Now},
	}

	cfg := l.managerConfig(&licenseSetup{
		key: "KW1-x", endpoint: "https://license.example.test",
	})

	tracking, ok := cfg.Client.HTTP.Transport.(trackingTransport)
	require.True(t, ok)
	require.True(t, tracking.next == http.RoundTripper(platform),
		"the license client must wrap the platform transport")
}

// useWorldClients points both outbound clients of deps at the fake license
// server: the license uses the platform client, providers the other.
func useWorldClients(deps *serverDeps, w *licenseWorld) {
	deps.clients.HTTP = w.srv.Client()
	deps.clients.PlatformHTTP = w.srv.Client()
}
