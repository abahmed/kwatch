package kube_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// restClient returns a real clientset served by a loopback handler, for
// code that needs a REST client the fake clientset does not provide.
func restClient(
	t *testing.T, h http.Handler,
) kubernetes.Interface {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
