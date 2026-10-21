package kubeclient

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
)

const (
	// DefaultHTTPTimeout bounds one outbound HTTP request.
	DefaultHTTPTimeout = 30 * time.Second
)

func defaultTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// NewHTTPClient builds the application-owned outbound client for alert
// providers and other destinations the operator chose, from config.
// Callers pass the returned client to components that perform HTTP
// requests; no process-wide mutable client is required. A proxy URL that
// does not parse or a CA bundle that cannot be used is an error: running
// without them would send provider traffic around the proxy or fail TLS
// later. app.insecureSkipTLSVerify applies to this client only.
func NewHTTPClient(cfg config.ApplicationRuntime) (*http.Client, error) {
	if cfg.InsecureSkipTLSVerify {
		klog.Warning(
			"outbound TLS certificate verification is disabled for " +
				"alert providers because insecureSkipTLSVerify is " +
				"enabled",
		)
	}
	return newHTTPClient(cfg, cfg.InsecureSkipTLSVerify)
}

// NewPlatformHTTPClient builds the client for kwatch's own services: the
// license server, the adoption telemetry and the update check. It follows
// the same proxy and CA bundle as the provider client but always verifies
// TLS: app.insecureSkipTLSVerify exists for the operator's own provider
// endpoints, and must never expose the license key or those calls to an
// impersonating server.
func NewPlatformHTTPClient(
	cfg config.ApplicationRuntime,
) (*http.Client, error) {
	return newHTTPClient(cfg, false)
}

func newHTTPClient(
	cfg config.ApplicationRuntime, insecure bool,
) (*http.Client, error) {
	transport := defaultTransport()

	if cfg.ProxyURL != "" {
		p, err := url.Parse(cfg.ProxyURL)
		if err != nil || p.Scheme == "" || p.Host == "" {
			// The parse error quotes the whole URL, which may carry proxy
			// credentials, so the message is fixed.
			return nil, errors.New("app.proxyURL is not a valid URL")
		}
		transport.Proxy = http.ProxyURL(p)
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: insecure, // #nosec G402
	}

	if cfg.CABundlePath != "" {
		if err := applyCABundle(tlsCfg, cfg.CABundlePath); err != nil {
			return nil, err
		}
	}

	transport.TLSClientConfig = tlsCfg
	return &http.Client{
		Timeout:   DefaultHTTPTimeout,
		Transport: transport,
	}, nil
}

// applyCABundle trusts the CA certificates in the file at path, in
// addition to the system roots. A missing or invalid bundle is an error.
func applyCABundle(tlsCfg *tls.Config, path string) error {
	caCert, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("app.caBundlePath cannot be read: %w", err)
	}
	// Keep the system roots so public endpoints (telemetry, update check,
	// SaaS providers) still verify next to a private bundle.
	pool, sysErr := x509.SystemCertPool()
	if sysErr != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(caCert) {
		return fmt.Errorf(
			"app.caBundlePath %s contains no valid certificates", path)
	}
	tlsCfg.RootCAs = pool
	return nil
}
