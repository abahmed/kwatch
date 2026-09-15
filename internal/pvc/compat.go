package pvc

import (
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

// NewPvcMonitor preserves the historical configuration and function-clock
// constructor for standalone callers.
func NewPvcMonitor(
	client kubernetes.Interface,
	monitorConfig *config.PvcMonitor,
	incidentSink monitor.ObservationSink,
	stateStore StateStore,
	clocks ...func() time.Time,
) *PvcMonitor {
	var normalized config.PvcMonitor
	if monitorConfig != nil {
		normalized = *monitorConfig
	}
	return newPvcMonitor(
		client, normalized, incidentSink, stateStore,
		clock.From(clocks),
	)
}

// NewPvcMonitorWithRuntime preserves the previous runtime constructor for
// embedded callers. Application composition uses the explicit-clock form.
func NewPvcMonitorWithRuntime(
	client kubernetes.Interface,
	runtime config.RuntimeConfig,
	incidentSink monitor.ObservationSink,
	stateStore StateStore,
	now func() time.Time,
) *PvcMonitor {
	return NewPvcMonitorWithRuntimeAndClock(
		client, runtime, incidentSink, stateStore, clock.Func(now),
	)
}

// SetNamespaceScope preserves the historical scope-wiring seam.
func (p *PvcMonitor) SetNamespaceScope(
	allowed, forbidden []string,
	all ...bool,
) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.setNamespaceScopeLocked(allowed, forbidden, all...)
}

// SetNamespaceFilter preserves the historical scope-wiring seam.
func (p *PvcMonitor) SetNamespaceFilter(filter func(string) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.namespaceFilter = filter
	p.pvByPVC = nil
	p.pvByPVCAt = time.Time{}
}
