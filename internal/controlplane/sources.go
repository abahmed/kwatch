package controlplane

import (
	"fmt"

	corev1lister "k8s.io/client-go/listers/core/v1"
)

// Sources are the controller-owned inputs used by control-plane checks.
type Sources struct {
	PodLister corev1lister.PodLister
	Resolver  HostResolver
}

// ConfigureSources freezes the control-plane source dependencies.
func (m *Monitor) ConfigureSources(sources Sources) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf(
			"control-plane sources cannot change after processing starts",
		)
	}
	if m.configured {
		return fmt.Errorf("control-plane sources are already configured")
	}
	m.configured = true
	m.podLister = sources.PodLister
	// Assign nil explicitly so a reused monitor cannot retain a stale resolver.
	m.resolver = sources.Resolver
	return nil
}
