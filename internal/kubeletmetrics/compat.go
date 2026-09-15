package kubeletmetrics

import (
	"time"

	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/observe"
)

// New preserves the historical function-clock constructor for embedded
// callers. Application composition uses NewWithClock.
func New(
	client kubernetes.Interface,
	cfg config.KubeletTelemetryMonitor,
	incidentSink monitor.ObservationSink,
	clocks ...func() time.Time,
) *Monitor {
	return NewWithClock(
		client, cfg, incidentSink, clock.Func(clock.From(clocks)),
	)
}

// SetNodeLister preserves the historical cache-wiring seam.
func (m *Monitor) SetNodeLister(lister corev1lister.NodeLister) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeLister = lister
}

// SetPodLister preserves the historical cache-wiring seam.
func (m *Monitor) SetPodLister(lister corev1lister.PodLister) {
	m.mu.Lock()
	m.podLister = lister
	m.mu.Unlock()
}

// SetStateStore preserves the historical persistence-wiring seam.
func (m *Monitor) SetStateStore(store StateStore) {
	m.store = store
}

// SetNamespaceScope preserves the historical scope-wiring seam.
func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.mu.Lock()
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
	m.mu.Unlock()
}

// SetOwnerResolver preserves the historical owner-wiring seam.
func (m *Monitor) SetOwnerResolver(owners observe.OwnerResolver) {
	m.mu.Lock()
	m.owners = owners
	m.mu.Unlock()
}

// SetNamespaceFilter preserves the historical scope-wiring seam.
func (m *Monitor) SetNamespaceFilter(allowed func(string) bool) {
	m.mu.Lock()
	m.allowed = allowed
	m.mu.Unlock()
}
