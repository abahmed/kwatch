package rbac

import (
	"encoding/json"
)

// Snapshot returns a copy so health readers cannot mutate monitor state.
func (m *Monitor) Snapshot() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	status.Missing = append([]Permission(nil), m.status.Missing...)
	status.State = securityState(status)
	return status
}

func (m *Monitor) SecurityStatus() Status {
	return m.Snapshot()
}

// StatusJSON implements the health status boundary.
func (m *Monitor) StatusJSON() ([]byte, error) {
	return json.Marshal(m.Snapshot())
}

func securityState(status Status) string {
	if status.RBACDenied {
		return "rbacDenied"
	}
	if !status.Available || status.LastCheck.IsZero() {
		return "unavailable"
	}
	return "healthy"
}
