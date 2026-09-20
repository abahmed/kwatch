package probe

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// Sources are the controller-owned inputs used by automatic probes.
type Sources struct {
	Kubernetes    kubernetes.Interface
	ServiceLister corev1lister.ServiceLister
	Graph         *kwcontext.ResourceGraph
	Namespaces    []string
	WatchAll      bool
	Allowed       func(string) bool
}

// ConfigureSources freezes the probe's controller dependencies.
func (m *Monitor) ConfigureSources(sources Sources) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("probe sources cannot change after processing starts")
	}
	if m.configured {
		return fmt.Errorf("probe sources are already configured")
	}
	m.configured = true
	m.kclient = sources.Kubernetes
	m.serviceLister = sources.ServiceLister
	m.graph = sources.Graph
	m.namespaces = append([]string(nil), sources.Namespaces...)
	m.watchAll = sources.WatchAll
	m.allowed = sources.Allowed
	return nil
}
