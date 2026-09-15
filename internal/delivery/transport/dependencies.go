package transport

import (
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

// Now returns the configured time for provider payloads and signatures. A
// zero value keeps compatibility-created providers deterministic; production
// composition always supplies the application clock.
func (d Dependencies) Now() time.Time {
	if d.Clock == nil {
		return time.Time{}
	}
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
