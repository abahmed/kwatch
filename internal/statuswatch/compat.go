package statuswatch

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"

	"github.com/abahmed/kwatch/internal/clock"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/monitor"
)

// NewWithClients preserves the historical function-clock constructor for
// standalone callers. Application composition uses NewWithClientsAndClock.
func NewWithClients(
	client dynamic.Interface,
	discoveryClient discovery.DiscoveryInterface,
	incidentSink monitor.ObservationSink,
	resync time.Duration,
	clocks ...func() time.Time,
) *Monitor {
	return NewWithClientsAndClock(
		client, discoveryClient, incidentSink, resync,
		clock.Func(clock.From(clocks)),
	)
}

// New is retained for callers that still own a REST configuration. The
// application uses NewWithClients so client construction remains centralized.
func New(
	restConfig *rest.Config,
	incidentSink monitor.ObservationSink,
	resync time.Duration,
	clocks ...func() time.Time,
) (*Monitor, error) {
	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("statuswatch: create dynamic client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf(
			"statuswatch: create discovery client: %w", err,
		)
	}
	return NewWithClients(
		client, discoveryClient, incidentSink, resync, clocks...,
	), nil
}

// SetServiceLister preserves the historical cache-wiring seam.
func (m *Monitor) SetServiceLister(lister corev1lister.ServiceLister) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serviceLister = lister
}

// SetGraph preserves the historical graph-wiring seam.
func (m *Monitor) SetGraph(graph *kwcontext.ResourceGraph) {
	m.graph = graph
}

// SetNamespaceFilter preserves the historical scope-wiring seam.
func (m *Monitor) SetNamespaceFilter(filter func(string) bool) {
	m.namespaceAllowed = filter
}

// SetNamespaceScope preserves the historical scope-wiring seam.
func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
}

// SetConditionRules preserves the historical policy-wiring seam.
func (m *Monitor) SetConditionRules(entries []string) error {
	rules := make(map[string]map[string]bool)
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" ||
			strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf(
				"invalid condition rule %q: use ConditionType=Status",
				entry,
			)
		}
		typ := strings.TrimSpace(parts[0])
		status := strings.TrimSpace(parts[1])
		if rules[typ] == nil {
			rules[typ] = make(map[string]bool)
		}
		rules[typ][status] = true
	}
	if len(rules) == 0 {
		return nil
	}
	m.conditionRules = rules
	return nil
}

// SetGraphReferenceRules preserves the historical policy-wiring seam.
func (m *Monitor) SetGraphReferenceRules(entries []string) error {
	rules := make([]graphReferenceRule, 0, len(entries))
	for _, entry := range entries {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" ||
			strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf(
				"invalid graph reference %q: use path=kind", entry,
			)
		}
		path := make([]string, 0)
		for _, part := range strings.Split(strings.TrimSpace(parts[0]), ".") {
			if part != "" {
				path = append(path, part)
			}
		}
		kind := strings.ToLower(strings.TrimSpace(parts[1]))
		if len(path) == 0 || kind == "" {
			return fmt.Errorf(
				"invalid graph reference %q: use path=kind", entry,
			)
		}
		rules = append(rules, graphReferenceRule{path: path, kind: kind})
	}
	m.graphReferences = rules
	return nil
}
