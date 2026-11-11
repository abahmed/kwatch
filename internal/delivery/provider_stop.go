package delivery

import (
	"github.com/abahmed/kwatch/internal/delivery/api"
)

// stopProviders ends the background work of a generation's providers
// once shutdown sent everything it could. A reconfiguration does not
// stop them: what the old providers still retry is bounded and belongs
// to alerts that are already over.
func (m *Manager) stopProviders(generation *providerGeneration) {
	if generation == nil || m.IsReconfiguring() {
		return
	}
	for _, name := range generation.order {
		stopper, ok := generation.entries[name].provider.(api.Stopper)
		if ok {
			stopper.Stop()
		}
	}
}
