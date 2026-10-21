package kubeclient

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
)

// app.insecureSkipTLSVerify is for the operator's own provider endpoints.
// The license server, the telemetry and the update check always verify.
func TestPlatformHTTPClientVerifiesTLSWhenProvidersDoNot(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	cfg := config.ApplicationRuntime{InsecureSkipTLSVerify: true}

	provider, err := NewHTTPClient(cfg)
	require.NoError(t, err)
	resp, err := provider.Get(srv.URL)
	require.NoError(t, err, "providers honour insecureSkipTLSVerify")
	_ = resp.Body.Close()

	platform, err := NewPlatformHTTPClient(cfg)
	require.NoError(t, err)
	_, err = platform.Get(srv.URL)
	require.Error(t, err, "an unverifiable certificate must be refused")
}

func TestPlatformHTTPClientHonoursTheProxyAndCABundle(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	bundle := t.TempDir() + "/ca.pem"
	require.NoError(t, os.WriteFile(bundle, clusterCA(srv), 0o600))

	platform, err := NewPlatformHTTPClient(config.ApplicationRuntime{
		CABundlePath: bundle,
	})
	require.NoError(t, err)
	resp, err := platform.Get(srv.URL)
	require.NoError(t, err, "a CA bundle must trust the server")
	_ = resp.Body.Close()

	_, err = NewPlatformHTTPClient(config.ApplicationRuntime{
		ProxyURL: "not a url",
	})
	require.Error(t, err)
}
