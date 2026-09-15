package statuswatch

import (
	"fmt"

	corev1lister "k8s.io/client-go/listers/core/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// Sources are the controller-owned inputs used by generic status watching.
type Sources struct {
	NamespaceAllowed func(string) bool
	Namespaces       []string
	WatchAll         bool
	Graph            *kwcontext.ResourceGraph
	ServiceLister    corev1lister.ServiceLister
}

// ConfigureSources freezes status-watch dependencies before Start.
func (m *Monitor) ConfigureSources(sources Sources) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("status sources cannot change after processing starts")
	}
	if m.configured {
		return fmt.Errorf("status sources are already configured")
	}
	m.configured = true
	m.namespaceAllowed = sources.NamespaceAllowed
	m.namespaces = append([]string(nil), sources.Namespaces...)
	m.watchAll = sources.WatchAll
	m.graph = sources.Graph
	m.serviceLister = sources.ServiceLister
	return nil
}
