package kubeclient

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// A kubelet that answers with a redirect must not be followed: the
// transport adds the ServiceAccount token to every request, so following
// would send it to whatever address the redirect names.
func TestKubeletClientDoesNotFollowRedirects(t *testing.T) {
	var elsewhere atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) { elsewhere.Add(1) }))
	defer target.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/stats/summary",
				http.StatusFound)
		}))
	defer srv.Close()
	base := &rest.Config{
		BearerToken:     "sa-token",
		TLSClientConfig: rest.TLSClientConfig{CAData: clusterCA(srv)},
	}
	c := newTestKubeletClient(t, base, true, kubeletNode(t, srv))

	_, err := readKubelet(t, c, "stats/summary")

	require.Error(t, err, "a redirect is not a successful answer")
	assert.Contains(t, err.Error(), "status 302")
	assert.Zero(t, elsewhere.Load(),
		"the redirect target must never receive the request")
}
