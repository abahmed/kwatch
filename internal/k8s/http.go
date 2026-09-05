package k8s

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
		p, err := url.Parse(cfg.ProxyURL)
		if err != nil || p.Scheme == "" || p.Host == "" {
			klog.ErrorS(err, "invalid outbound proxy URL")
		} else {
			transport.Proxy = http.ProxyURL(p)
		}
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipTLSVerify, // #nosec G402
	}
	if cfg.InsecureSkipTLSVerify {
		klog.Warning("InsecureSkipTLSVerify is enabled — outbound TLS certificate verification DISABLED")
	}

	if cfg.CABundlePath != "" {
		caCert, err := os.ReadFile(cfg.CABundlePath)
		if err != nil {
			klog.ErrorS(
				err, "could not read outbound CA bundle", "path", cfg.CABundlePath,
			)
		} else {
			caCertPool := x509.NewCertPool()
			if caCertPool.AppendCertsFromPEM(caCert) {
				tlsCfg.RootCAs = caCertPool
			} else {
				klog.Warning(
					"outbound CA bundle contains no valid certificates: ",
					cfg.CABundlePath,
				)
			}
		}
	}

	transport.TLSClientConfig = tlsCfg
	defaultClient.Transport = transport
}

// GetDefaultClient returns the shared default HTTP client.
func GetDefaultClient() *http.Client {
	return defaultClient
}
