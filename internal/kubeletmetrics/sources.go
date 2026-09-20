package kubeletmetrics

import (
	"fmt"

	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/observe"
)

// Sources are the controller-owned inputs used by kubelet telemetry.
type Sources struct {
	Namespaces    []string
	WatchAll      bool
	Allowed       func(string) bool
	OwnerResolver observe.OwnerResolver
	PodLister     corev1lister.PodLister
	NodeLister    corev1lister.NodeLister
	StateStore    StateStore
}

// ConfigureSources freezes the kubelet monitor's controller dependencies.
func (m *Monitor) ConfigureSources(sources Sources) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf(
			"kubelet sources cannot change after processing starts",
		)
	}
	if m.configured {
		return fmt.Errorf("kubelet sources are already configured")
	}
	m.configured = true
	m.namespaces = append([]string(nil), sources.Namespaces...)
	m.watchAll = sources.WatchAll
	m.allowed = sources.Allowed
	m.owners = sources.OwnerResolver
	m.podLister = sources.PodLister
	m.nodeLister = sources.NodeLister
	m.store = sources.StateStore
	return nil
}
