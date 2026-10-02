package incident

// RecordScope remembers whether the announcement of incident id was
// delivered. Updates and the resolve of a delivered incident stay in scope
// even after every finding that put it there has cleared.
func (m *Manager) RecordScope(id string, in bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil {
		p.Scope = ScopeOut
		p.sent.withdraw()
		if in {
			p.Scope = ScopeIn
			p.sent.confirm()
		}
	}
}

// HoldAnnouncement marks the announcement of incident id as collected for
// the startup summary. A held incident is persisted as not yet announced,
// so a restart before the summary is sent announces it again.
func (m *Manager) HoldAnnouncement(id string) {
	m.setHeld(id, false, true)
}

// ReleaseAnnouncement marks the held announcement of incident id as handed
// to delivery.
func (m *Manager) ReleaseAnnouncement(id string) {
	m.setHeld(id, true, false)
}

// setHeld moves the held flag. Holding withdraws the announcement's
// sent mark: nobody got it yet. Releasing confirms the latest decision,
// which is delivered right after the release.
func (m *Manager) setHeld(id string, from, to bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil && p.Held == from {
		p.Held = to
		if to {
			p.sent.withdraw()
		} else {
			p.sent.confirm()
		}
	}
}

// Closed reports whether none of the incidents ids is still live: each is
// resolved or already forgotten.
func (m *Manager) Closed(ids []string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if p := m.incidents[id]; p != nil && p.State != Resolved {
			return false
		}
	}
	return true
}
