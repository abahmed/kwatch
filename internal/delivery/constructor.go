package delivery

import (
	"net/http"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// Dependencies contains the process-owned collaborators used by delivery.
// Provider construction receives the same client through the catalog factory.
type Dependencies struct {
	HTTPClient *http.Client
	Clock      clock.Clock
}

// NewManagerWithDependencies constructs delivery with explicit dependencies.
func NewManagerWithDependencies(deps Dependencies) *Manager {
	now := clock.Require(deps.Clock)
	return &Manager{
		providerDeps: transport.Dependencies{
			HTTPClient: deps.HTTPClient,
			Clock:      deps.Clock,
			Sender: transport.NewSender(transport.Dependencies{
				HTTPClient: deps.HTTPClient,
				Clock:      deps.Clock,
			}),
		},
		now: now.Now,
	}
}
