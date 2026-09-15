package statuswatch

import (
	"encoding/json"
)

// Status is the safe diagnostic view of the generic status monitor.
type Status struct {
	State            string   `json:"state"`
	Started          bool     `json:"started"`
	SkippedResources []string `json:"skippedResources,omitempty"`
}

// Status returns watcher availability without exposing internal errors or
// Kubernetes client details.
func (m *Monitor) Status() Status {
	if m == nil {
		return Status{State: "unavailable"}
	}
	m.mu.Lock()
	started := m.started
	staticWatcher := m.staticWatcher
	m.mu.Unlock()
	if !started {
		return Status{State: "stopped"}
	}
	if staticWatcher == nil {
		return Status{State: "degraded", Started: true}
	}
	watcherStatus := staticWatcher.Status()
	state := watcherStatus.State
	if state == "unavailable" {
		state = "degraded"
	}
	return Status{
		State:            state,
		Started:          true,
		SkippedResources: append([]string(nil), watcherStatus.SkippedResources...),
	}
}

// StatusJSON implements the health diagnostic boundary.
func (m *Monitor) StatusJSON() ([]byte, error) {
	return json.Marshal(m.Status())
}
