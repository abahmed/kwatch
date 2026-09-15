package networkgraph

import (
	"fmt"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

// Sources are controller-owned scope inputs for the Gateway graph watcher.
type Sources struct {
	Allowed    func(string) bool
	Namespaces []string
	WatchAll   bool
	Graph      *kwcontext.ResourceGraph
}

// ConfigureSources freezes graph scope before the watcher starts.
func (m *Monitor) ConfigureSources(sources Sources) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.configured {
		return fmt.Errorf("network graph sources are already configured")
	}
	m.configured = true
	m.allowed = sources.Allowed
	m.namespaces = append([]string(nil), sources.Namespaces...)
	m.watchAll = sources.WatchAll
	if sources.Graph != nil {
		m.graph = sources.Graph
	}
	return nil
}
