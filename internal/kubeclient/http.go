package kubeclient

import (
	"crypto/tls"
	"crypto/x509"
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

// NewHTTPClient builds the application-owned outbound client from config.
// Callers pass the returned client to components that perform HTTP requests;
// no process-wide mutable client is required.
func NewHTTPClient(cfg config.ApplicationRuntime) *http.Client {
	transport := defaultTransport()

	if cfg.ProxyURL != "" {
		p, err := url.Parse(cfg.ProxyURL)
		if err != nil || p.Scheme == "" || p.Host == "" {
			// The parse error quotes the whole URL, which may carry proxy
			// credentials, so only a fixed message is logged.
			klog.ErrorS(nil, "invalid outbound proxy URL; proxy disabled")
		} else {
			transport.Proxy = http.ProxyURL(p)
		}
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipTLSVerify, // #nosec G402
	}
	if cfg.InsecureSkipTLSVerify {
		klog.Warning(
			"outbound TLS certificate verification is disabled " +
				"because insecureSkipTLSVerify is enabled",
		)
	}

	if cfg.CABundlePath != "" {
		applyCABundle(tlsCfg, cfg.CABundlePath)
	}

	transport.TLSClientConfig = tlsCfg
	return &http.Client{
		Timeout:   DefaultHTTPTimeout,
		Transport: transport,
	}
}

// applyCABundle trusts the CA certificates in the file at path, in
// addition to the system roots. A missing or invalid bundle is logged and
// the system roots stay in use.
func applyCABundle(tlsCfg *tls.Config, path string) {
	caCert, err := os.ReadFile(path)
	if err != nil {
		klog.ErrorS(err, "could not read outbound CA bundle", "path", path)
		return
	}
	// Keep the system roots so public endpoints (telemetry, update check,
	// SaaS providers) still verify next to a private bundle.
	pool, sysErr := x509.SystemCertPool()
	if sysErr != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(caCert) {
		klog.Warningf(
			"outbound CA bundle contains no valid certificates: %s", path)
		return
	}
	tlsCfg.RootCAs = pool
}
