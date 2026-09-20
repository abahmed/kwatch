package storagegraph

import (
	"fmt"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/monitor"
)

// Sources are controller-owned inputs for the storage graph watcher.
type Sources struct {
	Allowed         func(string) bool
	Namespaces      []string
	WatchAll        bool
	Graph           *kwcontext.ResourceGraph
	ObservationSink monitor.ObservationSink
}

// ConfigureSources freezes graph scope and observation dependencies.
func (m *Monitor) ConfigureSources(sources Sources) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.started {
		return fmt.Errorf("storage graph sources cannot change after start")
	}
	if m.configured {
		return fmt.Errorf("storage graph sources are already configured")
	}
	m.configured = true
	m.allowed = sources.Allowed
	m.namespaces = append([]string(nil), sources.Namespaces...)
	m.watchAll = sources.WatchAll
	if sources.Graph != nil {
		m.graph = sources.Graph
	}
	m.incidentSink = sources.ObservationSink
	return nil
}
