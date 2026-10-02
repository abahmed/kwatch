package kubeclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/config"
)

// A probe of a cluster-issued certificate succeeds with the cluster CA,
// and the provider proxy never sees probe traffic.
func TestProbeHTTPClientTrustsClusterCAWithoutProxy(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
	defer srv.Close()
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	client := NewProbeHTTPClient(&rest.Config{
		TLSClientConfig: rest.TLSClientConfig{CAData: clusterCA(srv)},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestProbeHTTPClientIsNotTheProviderClient(t *testing.T) {
	provider := NewHTTPClient(config.ApplicationRuntime{
		ProxyURL: "http://proxy.example:8080",
	})
	probe := NewProbeHTTPClient(nil)
	req := &http.Request{URL: &url.URL{Scheme: "https", Host: "svc"}}

	proxy, err := provider.Transport.(*http.Transport).Proxy(req)
	require.NoError(t, err)
	assert.Equal(t, "proxy.example:8080", proxy.Host)
	assert.Nil(t, probe.Transport.(*http.Transport).Proxy,
		"probes never use the outbound proxy")
}
