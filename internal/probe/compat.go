package probe

import (
	"net/http"
	"time"

	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/monitor"
)

// New is the compatibility constructor for callers that predate explicit
// resolver and clock dependencies. Application composition uses
// NewWithDependencies.
func New(
	cfg config.ActiveProbeMonitor,
	incidentSink monitor.ObservationSink,
	sharedClient *http.Client,
	clocks ...func() time.Time,
) *Monitor {
	if sharedClient == nil {
		sharedClient = &http.Client{}
	}
	return NewWithDependencies(
		cfg,
		incidentSink,
		sharedClient,
		nil,
		clock.Func(clock.From(clocks)),
	)
}

// SetResolver is retained for compatibility and focused unit tests. New
// production wiring supplies the resolver to NewWithDependencies.
func (m *Monitor) SetResolver(resolver HostResolver) {
	m.mu.Lock()
	m.resolver = resolver
	m.mu.Unlock()
}

// SetServiceLister preserves the historical cache-wiring seam.
func (m *Monitor) SetServiceLister(lister corev1lister.ServiceLister) {
	m.mu.Lock()
	m.serviceLister = lister
	m.mu.Unlock()
}

// SetKubernetesClient preserves the historical client-wiring seam.
func (m *Monitor) SetKubernetesClient(client kubernetes.Interface) {
	m.kclient = client
}

// SetGraph preserves the historical graph-wiring seam.
func (m *Monitor) SetGraph(graph *kwcontext.ResourceGraph) {
	m.graph = graph
}

// SetNamespaceScope preserves the historical namespace-wiring seam.
func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.mu.Lock()
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
	m.mu.Unlock()
}

// SetNamespaceFilter preserves the historical namespace-wiring seam.
func (m *Monitor) SetNamespaceFilter(allowed func(string) bool) {
	m.mu.Lock()
	m.allowed = allowed
	m.mu.Unlock()
}
