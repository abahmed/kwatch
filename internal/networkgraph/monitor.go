package networkgraph

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
)

type Monitor struct {
	client          dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	graph           *kwcontext.ResourceGraph
	resync          time.Duration
	allowed         func(string) bool
	namespaces      []string
	watchAll        bool
	dynamicWatcher  *dynamicwatch.Watcher
	generation      dynamicwatch.Generation
	configured      bool
	started         bool
	lifecycleMu     sync.Mutex
}

// NewWithClients constructs the Gateway graph monitor with shared clients.
func NewWithClients(
	client dynamic.Interface,
	discoveryClient discovery.DiscoveryInterface,
	graph *kwcontext.ResourceGraph,
	resync time.Duration,
) *Monitor {
	return &Monitor{
		client: client, discoveryClient: discoveryClient, graph: graph,
		resync: resync, watchAll: true,
	}
}

func (m *Monitor) Start(ctx context.Context) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.dynamicWatcher == nil {
		m.dynamicWatcher = dynamicwatch.NewWatcher(
			m.client,
			m.discoveryClient,
			m.resync,
			m.watchNamespaces,
			k8s.TrimManagedFields,
		)
	}
	generation, err := m.dynamicWatcher.StartGeneration(
		ctx, m.resourceSpecs(),
	)
	if err == nil && generation.Valid() {
		m.generation = generation
		m.started = true
	}
	return err
}

// Stop ends all Gateway API informers owned by the monitor.
func (m *Monitor) Stop(ctx context.Context) error {
	m.lifecycleMu.Lock()
	watcher := m.dynamicWatcher
	generation := m.generation
	m.started = false
	m.generation = dynamicwatch.Generation{}
	m.lifecycleMu.Unlock()
	if watcher != nil {
		if generation.Valid() {
			return generation.Stop(ctx)
		} else {
			return watcher.Stop(ctx)
		}
	}
	return nil
}

// Status reports optional Gateway API watcher health.
func (m *Monitor) Status() dynamicwatch.Status {
	if m == nil {
		return dynamicwatch.Status{State: "unavailable"}
	}
	m.lifecycleMu.Lock()
	watcher := m.dynamicWatcher
	m.lifecycleMu.Unlock()
	if watcher == nil {
		return dynamicwatch.Status{State: "unavailable"}
	}
	return watcher.Status()
}

// WaitForCacheSync reports whether the current optional watcher generation
// reached a usable cache state before the caller's context expired.
func (m *Monitor) WaitForCacheSync(ctx context.Context) bool {
	if m == nil {
		return false
	}
	m.lifecycleMu.Lock()
	watcher := m.dynamicWatcher
	generation := m.generation
	m.lifecycleMu.Unlock()
	if watcher == nil {
		return false
	}
	if generation.Valid() {
		return generation.WaitForCacheSync(ctx)
	}
	return watcher.WaitForCacheSync(ctx)
}

// StatusJSON implements the health status boundary.
func (m *Monitor) StatusJSON() ([]byte, error) {
	return json.Marshal(m.Status())
}
