package controlplane

import (
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
)

// NewWithREST preserves the historical function-clock constructor. The
// application uses NewWithRESTDependencies with an explicit clock and
// resolver.
func NewWithREST(
	restClient rest.Interface,
	client kubernetes.Interface,
	cfg config.ControlPlaneMonitor,
	incidentSink monitor.ObservationSink,
	clocks ...func() time.Time,
) *Monitor {
	return NewWithRESTDependencies(
		restClient, client, cfg, incidentSink, nil,
		clock.Func(clock.From(clocks)),
	)
}

// New is retained for callers that still own a REST configuration. The
// application uses NewWithREST so client construction is centralized.
func New(
	restConfig *rest.Config,
	client kubernetes.Interface,
	cfg config.ControlPlaneMonitor,
	incidentSink monitor.ObservationSink,
	clocks ...func() time.Time,
) (*Monitor, error) {
	restClient, err := rest.RESTClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("controlplane: create REST client: %w", err)
	}
	return NewWithREST(
		restClient, client, cfg, incidentSink, clocks...,
	), nil
}

// SetPodLister preserves the historical cache-wiring seam.
func (m *Monitor) SetPodLister(lister corev1lister.PodLister) {
	m.mu.Lock()
	m.podLister = lister
	m.mu.Unlock()
}

// SetResolver preserves the historical resolver-wiring seam.
func (m *Monitor) SetResolver(resolver HostResolver) {
	m.mu.Lock()
	m.resolver = resolver
	m.mu.Unlock()
}
