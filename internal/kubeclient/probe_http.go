package kubeclient

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// probeHTTPTimeout is a safety net; the prober bounds each check with its
// own shorter timeout.
const probeHTTPTimeout = 30 * time.Second

// NewProbeHTTPClient builds the client for active probes of in-cluster
// endpoints. It is separate from the provider client on purpose: probes
// must reach Services directly, so the outbound proxy and the provider CA
// bundle do not apply. Certificates are verified against the system roots
// plus the cluster CA, so endpoints with cluster-issued certificates work.
func NewProbeHTTPClient(restConfig *rest.Config) *http.Client {
	transport := defaultTransport()
	// Probes never go through an HTTP proxy.
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    probeRootCAs(restConfig),
	}
	return &http.Client{Timeout: probeHTTPTimeout, Transport: transport}
}

// probeRootCAs is the system pool with the cluster CA added. A missing or
// unreadable cluster CA leaves the system roots alone.
func probeRootCAs(restConfig *rest.Config) *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if restConfig == nil {
		return pool
	}
	ca := restConfig.TLSClientConfig.CAData
	if len(ca) == 0 && restConfig.TLSClientConfig.CAFile != "" {
		data, err := os.ReadFile(restConfig.TLSClientConfig.CAFile)
		if err != nil {
			klog.V(2).InfoS("cluster CA unavailable for probes",
				"component", "probe", "operation", "client")
			return pool
		}
		ca = data
	}
	if len(ca) > 0 {
		pool.AppendCertsFromPEM(ca)
	}
	return pool
}
