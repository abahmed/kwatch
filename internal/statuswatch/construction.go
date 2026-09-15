package statuswatch

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
	"github.com/abahmed/kwatch/internal/monitor"
)

// NewWithClients constructs status watching from application-owned clients.
// NewWithClientsAndClock constructs status watching with an explicit clock.
func NewWithClientsAndClock(
	client dynamic.Interface,
	discoveryClient discovery.DiscoveryInterface,
	incidentSink monitor.ObservationSink,
	resync time.Duration,
	timeSource clock.Clock,
) *Monitor {
	if timeSource == nil {
		timeSource = clock.RealClock{}
	}
	statusMonitor := &Monitor{
		client:            client,
		discoveryClient:   discoveryClient,
		incidentSink:      incidentSink,
		resync:            resync,
		factories:         make(map[string]dynamicwatch.Factory),
		stops:             make(map[string]context.CancelFunc),
		crdVersions:       make(map[string]map[string]struct{}),
		conditionRules:    defaultConditionRules(),
		now:               timeSource.Now,
		admissionPolicies: make(map[string]struct{}),
		admissionBindings: make(
			map[string]*unstructured.Unstructured,
		),
		watchAll: true,
	}
	statusMonitor.staticWatcher = dynamicwatch.NewWatcher(
		client, discoveryClient, resync,
		statusMonitor.watchNamespaces,
		k8s.TrimManagedFields,
	)
	return statusMonitor
}

func (m *Monitor) nowTime() time.Time {
	return m.now()
}
