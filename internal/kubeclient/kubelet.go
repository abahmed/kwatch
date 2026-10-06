package kubeclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// DefaultKubeletPort is the kubelet HTTPS port used when a Node does not
// report status.daemonEndpoints.kubeletEndpoint.Port.
const DefaultKubeletPort = 10250

const (
	// kubeletEndpointTTL is how long a resolved node address is reused
	// before the Node is read again. Node addresses rarely change; a
	// failed request also forgets the address at once.
	kubeletEndpointTTL = 10 * time.Minute
	// kubeletHTTPTimeout is a safety net; callers pass a shorter context.
	kubeletHTTPTimeout = 30 * time.Second
	// maxKubeletErrorBody bounds how much of an error response is drained
	// so the connection can be reused.
	maxKubeletErrorBody = 4 << 10
)

// errNoInternalIP reports a Node without an InternalIP address.
var errNoInternalIP = errors.New("node has no InternalIP address")

// KubeletClientConfig configures a KubeletClient.
type KubeletClientConfig struct {
	// Nodes reads Node objects to find each kubelet's address and port.
	Nodes kubernetes.Interface
	// Transport authenticates with the kwatch credentials and verifies
	// the kubelet serving certificate (see NewKubeletTransport).
	Transport http.RoundTripper
	Now       func() time.Time
}

// KubeletClient reads kubelet HTTPS endpoints such as /stats/summary
// directly from each node, at the node's InternalIP and kubelet port. It
// needs get on nodes/stats and nodes/metrics, not nodes/proxy.
type KubeletClient struct {
	nodes kubernetes.Interface
	http  *http.Client
	now   func() time.Time

	mu        sync.Mutex
	endpoints map[string]kubeletEndpoint
}

// kubeletEndpoint is one node's resolved kubelet address.
type kubeletEndpoint struct {
	hostPort string
	resolved time.Time
}

// NewKubeletClient builds a client. Every dependency is required.
func NewKubeletClient(cfg KubeletClientConfig) (*KubeletClient, error) {
	if cfg.Nodes == nil || cfg.Transport == nil || cfg.Now == nil {
		return nil, errors.New("kubelet client: incomplete configuration")
	}
	return &KubeletClient{
		nodes: cfg.Nodes,
		http: &http.Client{
			Transport: cfg.Transport, Timeout: kubeletHTTPTimeout,
		},
		now:       cfg.Now,
		endpoints: map[string]kubeletEndpoint{},
	}, nil
}

// Open starts a GET of path (such as "stats/summary") on node's kubelet.
// The caller closes the body and bounds how much of it is read. A non-200
// response is an error.
func (c *KubeletClient) Open(
	ctx context.Context, node, path string,
) (io.ReadCloser, error) {
	hostPort, err := c.endpoint(ctx, node)
	if err != nil {
		return nil, err
	}
	target := url.URL{Scheme: "https", Host: hostPort, Path: "/" + path}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// The node may have a new address; read it again next time.
		c.forget(node)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard,
			io.LimitReader(resp.Body, maxKubeletErrorBody))
		_ = resp.Body.Close()
		return nil, &KubeletStatusError{
			Node: node, Path: path, Code: resp.StatusCode,
		}
	}
	return resp.Body, nil
}

// endpoint returns node's kubelet host:port, reading the Node when the
// cached address is missing or older than kubeletEndpointTTL.
func (c *KubeletClient) endpoint(
	ctx context.Context, node string,
) (string, error) {
	now := c.now()
	c.mu.Lock()
	cached, ok := c.endpoints[node]
	c.mu.Unlock()
	if ok && now.Sub(cached.resolved) < kubeletEndpointTTL {
		return cached.hostPort, nil
	}
	object, err := c.nodes.CoreV1().Nodes().Get(
		ctx, node, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	hostPort, err := KubeletAddress(object)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dropExpiredLocked(now)
	c.endpoints[node] = kubeletEndpoint{hostPort: hostPort, resolved: now}
	return hostPort, nil
}

// dropExpiredLocked forgets addresses of nodes no poll asked for within
// the TTL, so deleted nodes do not stay in memory.
func (c *KubeletClient) dropExpiredLocked(now time.Time) {
	for name, e := range c.endpoints {
		if now.Sub(e.resolved) >= kubeletEndpointTTL {
			delete(c.endpoints, name)
		}
	}
}

func (c *KubeletClient) forget(node string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.endpoints, node)
}

// KubeletAddress returns the host:port of a Node's kubelet: its first
// InternalIP and the advertised kubelet port, or DefaultKubeletPort.
func KubeletAddress(node *corev1.Node) (string, error) {
	port := int(node.Status.DaemonEndpoints.KubeletEndpoint.Port)
	if port <= 0 {
		port = DefaultKubeletPort
	}
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP && address.Address != "" {
			return net.JoinHostPort(address.Address, strconv.Itoa(port)),
				nil
		}
	}
	return "", fmt.Errorf("%w: %s", errNoInternalIP, node.Name)
}

// NewKubeletTransport builds the HTTPS transport for kubelet requests from
// the API server configuration. It sends the same credentials (the
// ServiceAccount token, re-read when it rotates) and verifies the kubelet
// serving certificate with the cluster CA. insecureSkipVerify disables
// that check, for kubelets with self-signed serving certificates. Kubelet
// traffic never goes through an HTTP proxy.
func NewKubeletTransport(
	base *rest.Config, insecureSkipVerify bool,
) (http.RoundTripper, error) {
	if base == nil {
		return nil, errors.New("kubelet transport: no API configuration")
	}
	cfg := rest.CopyConfig(base)
	// The API server name does not apply to kubelet certificates.
	cfg.TLSClientConfig.ServerName = ""
	cfg.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	if insecureSkipVerify {
		klog.Warning("kubelet.insecureSkipVerify is true: kwatch sends its " +
			"ServiceAccount token to kubelets without verifying their " +
			"certificates, so a compromised or impersonated node could " +
			"capture that token. Prefer fixing the kubelet serving " +
			"certificate or its CA over keeping this setting")
		// client-go refuses a CA together with the insecure flag.
		cfg.TLSClientConfig.Insecure = true
		cfg.TLSClientConfig.CAFile = ""
		cfg.TLSClientConfig.CAData = nil
	}
	return rest.TransportFor(cfg)
}

// KubeletStatusError is a kubelet answer other than 200 OK. Callers read
// the code through StatusCode to tell a refusal (401, 403) from a failure.
type KubeletStatusError struct {
	Node string
	Path string
	Code int
}

func (e *KubeletStatusError) Error() string {
	return fmt.Sprintf("kubelet %s/%s: status %d", e.Node, e.Path, e.Code)
}

// StatusCode returns the HTTP status the kubelet answered with.
func (e *KubeletStatusError) StatusCode() int { return e.Code }
