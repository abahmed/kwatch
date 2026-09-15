package metricsapi

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/observe"
)

// New is retained for callers that still own a REST configuration. The
// application uses NewWithClient so dynamic client construction is centralized.
func New(
	restConfig *rest.Config,
	client kubernetes.Interface,
	cfg config.RuntimeMetricsMonitor,
	incidentSink monitor.ObservationSink,
) (*Monitor, error) {
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("metricsapi: create dynamic client: %w", err)
	}
	return NewWithClient(dynamicClient, client, cfg, incidentSink), nil
}

// SetPodLister preserves the historical cache-wiring seam.
func (m *Monitor) SetPodLister(lister corev1lister.PodLister) {
	m.podLister = lister
}

// SetOwnerResolver preserves the historical owner-wiring seam.
func (m *Monitor) SetOwnerResolver(owners observe.OwnerResolver) {
	m.owners = owners
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
