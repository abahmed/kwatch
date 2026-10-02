package kubeclient

import (
	"context"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// kubeletServer is a TLS server that answers like a kubelet and checks
// the bearer token.
func kubeletServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			if r.Header.Get("Authorization") != "Bearer sa-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.URL.Path != "/stats/summary" {
				http.NotFound(w, r)
				return
			}
			_, _ = io.WriteString(w, `{"node":{}}`)
		}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// kubeletNode is a Node whose InternalIP and kubelet port point at srv.
func kubeletNode(t *testing.T, srv *httptest.Server) *corev1.Node {
	t.Helper()
	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	n, err := strconv.Atoi(port)
	require.NoError(t, err)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Status.Addresses = []corev1.NodeAddress{
		{Type: corev1.NodeHostName, Address: "n1"},
		{Type: corev1.NodeInternalIP, Address: host},
	}
	node.Status.DaemonEndpoints.KubeletEndpoint.Port = int32(n)
	return node
}

// clusterCA returns srv's certificate as PEM, standing in for a cluster
// CA that signed the kubelet serving certificate.
func clusterCA(srv *httptest.Server) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: srv.Certificate().Raw,
	})
}

func newTestKubeletClient(
	t *testing.T, base *rest.Config, insecure bool, nodes ...*corev1.Node,
) *KubeletClient {
	t.Helper()
	transport, err := NewKubeletTransport(base, insecure)
	require.NoError(t, err)
	client := fake.NewClientset()
	for _, node := range nodes {
		_, err := client.CoreV1().Nodes().Create(
			context.Background(), node, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	kubelet, err := NewKubeletClient(KubeletClientConfig{
		Nodes: client, Transport: transport,
		Now: func() time.Time { return time.Unix(0, 0) },
	})
	require.NoError(t, err)
	return kubelet
}

func readKubelet(
	t *testing.T, c *KubeletClient, path string,
) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	body, err := c.Open(ctx, "n1", path)
	if err != nil {
		return "", err
	}
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	return string(data), err
}

func TestKubeletClientVerifiesWithClusterCA(t *testing.T) {
	srv, calls := kubeletServer(t)
	base := &rest.Config{
		BearerToken:     "sa-token",
		TLSClientConfig: rest.TLSClientConfig{CAData: clusterCA(srv)},
	}
	c := newTestKubeletClient(t, base, false, kubeletNode(t, srv))

	body, err := readKubelet(t, c, "stats/summary")
	require.NoError(t, err)
	assert.JSONEq(t, `{"node":{}}`, body)

	// The address is cached: a second read does not need the Node.
	_, err = readKubelet(t, c, "stats/summary")
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
}

func TestKubeletClientRejectsUntrustedCertificate(t *testing.T) {
	srv, calls := kubeletServer(t)
	base := &rest.Config{BearerToken: "sa-token"}
	c := newTestKubeletClient(t, base, false, kubeletNode(t, srv))

	_, err := readKubelet(t, c, "stats/summary")
	require.Error(t, err, "a certificate the CA did not sign is refused")
	assert.Zero(t, calls.Load(), "no request reaches the kubelet")
}

func TestKubeletClientInsecureSkipVerify(t *testing.T) {
	srv, _ := kubeletServer(t)
	// A CA that did not sign the kubelet certificate: insecure mode must
	// drop it rather than fail.
	other := httptest.NewTLSServer(http.NotFoundHandler())
	defer other.Close()
	base := &rest.Config{
		BearerToken:     "sa-token",
		TLSClientConfig: rest.TLSClientConfig{CAData: clusterCA(other)},
	}
	c := newTestKubeletClient(t, base, true, kubeletNode(t, srv))

	body, err := readKubelet(t, c, "stats/summary")
	require.NoError(t, err)
	assert.JSONEq(t, `{"node":{}}`, body)
}

func TestKubeletClientReportsErrorStatus(t *testing.T) {
	srv, _ := kubeletServer(t)
	base := &rest.Config{
		BearerToken:     "sa-token",
		TLSClientConfig: rest.TLSClientConfig{CAData: clusterCA(srv)},
	}
	c := newTestKubeletClient(t, base, false, kubeletNode(t, srv))

	_, err := readKubelet(t, c, "metrics/unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
	var status *KubeletStatusError
	require.ErrorAs(t, err, &status)
	assert.Equal(t, http.StatusNotFound, status.StatusCode())
}

func TestKubeletClientUnknownNode(t *testing.T) {
	c := newTestKubeletClient(t, &rest.Config{}, false)

	_, err := readKubelet(t, c, "stats/summary")
	require.Error(t, err)
}

func TestKubeletAddress(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Status.Addresses = []corev1.NodeAddress{
		{Type: corev1.NodeExternalIP, Address: "203.0.113.9"},
		{Type: corev1.NodeInternalIP, Address: "10.0.0.7"},
	}
	got, err := KubeletAddress(node)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.7:10250", got, "default port")

	node.Status.DaemonEndpoints.KubeletEndpoint.Port = 10255
	got, err = KubeletAddress(node)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.7:10255", got)

	node.Status.Addresses = node.Status.Addresses[:1]
	_, err = KubeletAddress(node)
	assert.ErrorIs(t, err, errNoInternalIP)
}

func TestNewKubeletClientRequiresDependencies(t *testing.T) {
	_, err := NewKubeletClient(KubeletClientConfig{})
	assert.Error(t, err)
	_, err = NewKubeletTransport(nil, false)
	assert.Error(t, err)
}
