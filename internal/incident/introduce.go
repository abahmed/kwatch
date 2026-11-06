package incident

// Introduce decides the first own message of incident id when a listing
// (a roll-up or a namespace outage) only named it: the listing says that
// it fails, not why, and nothing else may ever change enough to send an
// update that would say so. It reports false for an incident that is
// gone, resolved or quiet, was never carried by a listing, or no longer
// announced. The caller sends it at most once (pipeline introduce.go).
func (m *Manager) Introduce(id string) (Decision, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	if p == nil || p.quiet || !wasAnnounced(p) || !p.Delivery.RolledUp() {
		return Decision{}, false
	}
	return m.decide(p, Update, ReasonIntroduced), true
}

// AwaitsListing reports an open incident a listing has taken but not
// sent yet: a namespace outage holds its members for a while before
// its message goes out (RecordRolledUp then marks them). Its
// introduction waits for that, within the caller's bound.
func (m *Manager) AwaitsListing(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	return p != nil && !p.quiet && p.State != Resolved &&
		!p.Delivery.RolledUp()
}
