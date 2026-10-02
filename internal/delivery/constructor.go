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
	// OnDelivered, when set, is called after a provider accepts a job. The
	// application uses it to persist provider thread ids without waiting
	// for the periodic save. It must not block.
	OnDelivered func()
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
		now:               now.Now,
		onDelivered:       deps.OnDelivered,
		done:              doneSignal{ch: make(chan struct{})},
		reconfigureEvents: make(chan struct{}, 1),
	}
}
