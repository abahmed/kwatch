package kube_test

import (
	"context"
	"fmt"
	"io"
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

// handlerKubelet serves kubelet reads from a handler, at the path
// "/<node>/<endpoint>", standing in for the direct kubelet client.
type handlerKubelet struct{ h http.Handler }

func (k handlerKubelet) Open(
	_ context.Context, node, path string,
) (io.ReadCloser, error) {
	return serveKubelet(k.h, node, path)
}

func serveKubelet(
	h http.Handler, node, path string,
) (io.ReadCloser, error) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+node+"/"+path, nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return nil, fmt.Errorf("kubelet status %d", rec.Code)
	}
	return io.NopCloser(rec.Body), nil
}
