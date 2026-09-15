package rbac

import (
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

// New is retained for embedded callers that want the complete capability
// audit. NewWithRuntimeConfig is the canonical construction path.
func New(client kubernetes.Interface, clocks ...func() time.Time) *Monitor {
	return &Monitor{
		client:                client,
		clusterPermissions:    clusterPermissions(),
		namespacedPermissions: namespacedPermissions(),
		now:                   clock.From(clocks),
	}
}

// NewWithConfig is retained for callers that still provide YAML
// configuration. NewWithRuntimeConfig is the canonical construction path.
func NewWithConfig(
	client kubernetes.Interface,
	cfg *config.Config,
	clocks ...func() time.Time,
) *Monitor {
	return newConfiguredMonitor(
		client, config.RuntimeConfigFor(cfg), clock.From(clocks), cfg == nil,
	)
}

// permissionsForConfig is retained for package-level compatibility tests.
func permissionsForConfig(cfg *config.Config) ([]Permission, []Permission) {
	if cfg == nil {
		return clusterPermissions(), namespacedPermissions()
	}
	return permissionsForRuntime(config.RuntimeConfigFor(cfg))
}

// infrastructurePermissions is retained for package-level compatibility
// tests.
func infrastructurePermissions(cfg *config.Config) []Permission {
	if cfg == nil {
		return infrastructurePermissionsForRuntime(config.RuntimeConfig{})
	}
	return infrastructurePermissionsForRuntime(config.RuntimeConfigFor(cfg))
}

// SetNamespaces is retained for embedded callers using the retired setter
// wiring. Production composition uses ConfigureSources.
func (m *Monitor) SetNamespaces(namespaces []string) {
	m.mu.Lock()
	m.namespaces = append([]string(nil), namespaces...)
	m.mu.Unlock()
}

// SetAllNamespaces is retained for compatibility callers.
func (m *Monitor) SetAllNamespaces(all bool) {
	m.mu.Lock()
	m.allNamespaces = all
	m.mu.Unlock()
}

// SetInfrastructureNamespace is retained for compatibility callers.
func (m *Monitor) SetInfrastructureNamespace(namespace string) {
	m.mu.Lock()
	for i := range m.infrastructure {
		m.infrastructure[i].Namespace = namespace
	}
	m.mu.Unlock()
}
