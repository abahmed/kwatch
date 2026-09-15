package rbac

import "fmt"

// Sources contains the application-owned scope for permission checks.
// ConfigureSources freezes this bundle before the monitor starts.
type Sources struct {
	Namespaces              []string
	AllNamespaces           bool
	InfrastructureNamespace string
}

// ConfigureSources installs the complete RBAC source scope exactly once.
// A partially configured monitor is difficult to diagnose, so callers must
// provide all scope values in one operation before Start.
func (m *Monitor) ConfigureSources(sources Sources) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("rbac sources cannot change after start")
	}
	if m.configured {
		return fmt.Errorf("rbac sources are already configured")
	}
	m.namespaces = append([]string(nil), sources.Namespaces...)
	m.allNamespaces = sources.AllNamespaces
	for i := range m.infrastructure {
		m.infrastructure[i].Namespace = sources.InfrastructureNamespace
	}
	m.configured = true
	return nil
}
