package networkgraph

import (
	"fmt"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// New is retained for callers that still own a REST configuration. The
// application uses NewWithClients so client construction remains centralized.
func New(
	restConfig *rest.Config,
	graph *kwcontext.ResourceGraph,
	resync time.Duration,
) (*Monitor, error) {
	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("networkgraph: create dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("networkgraph: create discovery client: %w", err)
	}
	return NewWithClients(client, discoveryClient, graph, resync), nil
}

// SetNamespaceFilter preserves the historical scope-wiring seam.
func (m *Monitor) SetNamespaceFilter(filter func(string) bool) {
	m.allowed = filter
}

// SetNamespaceScope preserves the historical scope-wiring seam.
func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
}
