package metricsapi

import (
	"fmt"

	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/observe"
)

// Sources are the controller-owned inputs used by metrics-server monitoring.
type Sources struct {
	PodLister     corev1lister.PodLister
	OwnerResolver observe.OwnerResolver
	Allowed       func(string) bool
	Namespaces    []string
	WatchAll      bool
}

// ConfigureSources freezes the metrics monitor's controller dependencies.
func (m *Monitor) ConfigureSources(sources Sources) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("metrics sources cannot change after processing starts")
	}
	if m.configured {
		return fmt.Errorf("metrics sources are already configured")
	}
	m.configured = true
	m.podLister = sources.PodLister
	m.owners = sources.OwnerResolver
	m.allowed = sources.Allowed
	m.namespaces = append([]string(nil), sources.Namespaces...)
	m.watchAll = sources.WatchAll
	return nil
}
