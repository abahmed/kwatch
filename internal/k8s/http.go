package k8s

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/abahmed/kwatch/internal/config"
)

const (
	DefaultHTTPTimeout = 30 * time.Second
)

var defaultClient *http.Client

func init() {
	defaultClient = &http.Client{Timeout: DefaultHTTPTimeout}
}

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

// InitHTTPClient configures the shared HTTP transport based on app config.
// Must be called before any provider sends messages.
func InitHTTPClient(cfg *config.App) {
	transport := defaultTransport()

	if cfg.ProxyURL != "" {
		if p, err := url.Parse(cfg.ProxyURL); err == nil {
			transport.Proxy = http.ProxyURL(p)
		}
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipTLSVerify, // #nosec G402
	}

	if cfg.CABundlePath != "" {
		if caCert, err := os.ReadFile(cfg.CABundlePath); err == nil {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCert)
			tlsCfg.RootCAs = caCertPool
		}
	}

	transport.TLSClientConfig = tlsCfg
	defaultClient.Transport = transport
}

// NewHTTPClient returns an *http.Client using the shared transport.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   DefaultHTTPTimeout,
		Transport: defaultClient.Transport,
	}
}

// GetDefaultClient returns the shared default HTTP client.
func GetDefaultClient() *http.Client {
	return defaultClient
}
