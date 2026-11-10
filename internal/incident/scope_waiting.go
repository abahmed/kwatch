package incident

// An announcement the scope refused only for now (a Namespace whose
// labels kwatch has not read yet) is unheard like one out of scope,
// which a health page cannot tell apart. ScopeWaiting counts them.

// noteScopeWaiting marks incident id as waiting for the scope, or not.
// The caller holds m.mu.
func (m *Manager) noteScopeWaiting(id string, waiting bool) {
	if !waiting {
		delete(m.scopeWaiting, id)
		return
	}
	if m.scopeWaiting == nil {
		m.scopeWaiting = map[string]bool{}
	}
	m.scopeWaiting[id] = true
}

// ScopeWaiting counts the failing incidents whose announcement waits
// for the scope's verdict: unheard, but not out of scope.
func (m *Manager) ScopeWaiting() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for id := range m.scopeWaiting {
		p := m.incidents[id]
		if p == nil || !unheardFailing(p) {
			delete(m.scopeWaiting, id)
			continue
		}
		n++
	}
	return n
}

// RecordScopeDecided notes that the scope decided incident id, which
// was waiting for it, out of scope for good: no longer waiting.
func (m *Manager) RecordScopeDecided(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.noteScopeWaiting(id, false)
}
