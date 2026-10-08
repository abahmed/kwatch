package incident

// Restored reports whether incident id was loaded from the state file of
// an earlier run. The announce layer reads it: what such an incident does
// right after a restart is folded into the restored-incidents summary,
// not posted one by one.
func (m *Manager) Restored(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	return p != nil && p.restored
}
