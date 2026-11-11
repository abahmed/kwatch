package kubeclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestCollectionOfNamesListedAndWatchedResources(t *testing.T) {
	for path, want := range map[string][2]string{
		"/api/v1/pods":                             {"", "pods"},
		"/api/v1/namespaces":                       {"", "namespaces"},
		"/api/v1/namespaces/shop/pods":             {"", "pods"},
		"/apis/apps/v1/deployments":                {"apps", "deployments"},
		"/apis/apps/v1/namespaces/shop/daemonsets": {"apps", "daemonsets"},
		"/k8s/clusters/c1/apis/storage.k8s.io/v1/storageclasses": {
			"storage.k8s.io", "storageclasses"},
	} {
		group, resource, ok := collectionOf(path)
		assert.True(t, ok, path)
		assert.Equal(t, want, [2]string{group, resource}, path)
	}
	for _, path := range []string{
		"/api/v1/namespaces/shop", "/api/v1/namespaces/shop/pods/web-1",
		"/api/v1/namespaces/shop/pods/web-1/log", "/api/v1/nodes/n1",
		"/apis/apps/v1", "/api", "/version", "/healthz", "",
	} {
		_, _, ok := collectionOf(path)
		assert.False(t, ok, path)
	}
}

type fixedTripper struct {
	status int
	err    error
}

func (f fixedTripper) RoundTrip(*http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{StatusCode: f.status, Body: http.NoBody}, nil
}

func TestAnswersCountOnlySuccessfulListsAndWatches(t *testing.T) {
	a := &Answers{}
	get := func(rt http.RoundTripper, method, url string) {
		req := httptest.NewRequest(method, url, nil)
		resp, err := a.Wrap(rt).RoundTrip(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}
	ok := fixedTripper{status: http.StatusOK}
	get(ok, http.MethodGet,
		"https://api/api/v1/pods?limit=500&resourceVersion=0")
	get(ok, http.MethodGet, "https://api/api/v1/pods?watch=true")
	get(ok, http.MethodPost, "https://api/api/v1/pods")
	get(ok, http.MethodGet, "https://api/api/v1/nodes/n1")
	get(fixedTripper{status: http.StatusForbidden}, http.MethodGet,
		"https://api/apis/storage.k8s.io/v1/storageclasses")
	get(fixedTripper{err: errors.New("refused")}, http.MethodGet,
		"https://api/api/v1/pods")

	// Lists another client makes, naming no resource version, are not
	// the informer's.
	get(ok, http.MethodGet, "https://api/api/v1/namespaces/shop/pods")
	get(ok, http.MethodGet, "https://api/api/v1/pods?limit=1")

	assert.Equal(t, uint64(2), a.Count("", "pods"))
	assert.Zero(t, a.Count("", "nodes"))
	assert.Zero(t, a.Count("storage.k8s.io", "storageclasses"))
	assert.Zero(t, (*Answers)(nil).Count("", "pods"))
}

// The cluster clients are built over a configuration that counts their
// answers: a list through the typed client is counted.
func TestClusterClientsCountTheirAnswers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(
				`{"kind":"NamespaceList","apiVersion":"v1","items":[]}`))
		}))
	defer server.Close()
	clients, err := newClusterClients(&rest.Config{Host: server.URL})
	require.NoError(t, err)
	require.NotNil(t, clients.Answers)
	_, err = clients.Kubernetes.CoreV1().Namespaces().List(
		t.Context(), metav1.ListOptions{ResourceVersion: "0"})
	require.NoError(t, err)
	assert.Equal(t, uint64(1), clients.Answers.Count("", "namespaces"))
}
