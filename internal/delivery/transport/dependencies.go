package transport

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

// Dependencies contains the outbound dependencies shared by providers.
//
// HTTPClient is owned by application composition. Sender is the preferred
// seam for providers that need to control request metadata directly.
type Dependencies struct {
	HTTPClient *http.Client
	Sender     Sender
	Clock      clock.Clock
}

// ProviderContext is the complete immutable dependency set given to one
// provider during construction. ClusterName is application identity; the
// embedded dependencies are shared outbound infrastructure.
type ProviderContext struct {
	ClusterName string
	Dependencies
}

// Now returns the configured time for provider payloads and signatures.
// Application composition must always provide the clock.
func (d Dependencies) Now() time.Time {
	return d.Clock.Now()
}

// NewSender resolves the provider's request sender once during construction.
// Send paths retain that value instead of constructing a transport per call.
func NewSender(dependencies Dependencies) Sender {
	if dependencies.Sender != nil {
		return dependencies.Sender
	}
	return NewWithDependencies(dependencies)
}

// TLSConfig returns a copy of the TLS settings (extra root CAs, skip
// verify) carried by the application-owned HTTP client, or nil when it has
// none. Providers that speak TLS without HTTP, such as SMTP, use it so they
// honor the same trust configuration. The application's proxy is an HTTP
// proxy and does not apply to them.
func (d Dependencies) TLSConfig() *tls.Config {
	if d.HTTPClient == nil {
		return nil
	}
	base, ok := d.HTTPClient.Transport.(*http.Transport)
	if !ok || base.TLSClientConfig == nil {
		return nil
	}
	return base.TLSClientConfig.Clone()
}
