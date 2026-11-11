package incident

import "time"

// An announcement the scope dropped (a maintenance window, a silence, a
// namespace people filtered out) leaves an incident that is announced
// in kwatch's books and unheard by people (ScopeOut). Scope is only
// asked when there is a decision, and an unchanged incident decides
// nothing until its reminder, up to a week later. A failure that
// outlives its maintenance window would say nothing all that time. So
// the pipeline asks again on every tick, for the incidents listed here,
// and announces the one whose scope came back.

// Unheard returns the live incidents whose announcement the scope
// dropped and that still fail: the ones to ask the scope about again.
func (m *Manager) Unheard() []Incident {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Incident
	for _, id := range m.liveIDs() {
		if p := m.incidents[id]; unheardFailing(p) {
			out = append(out, p.Snapshot())
		}
	}
	return out
}

// unheardFailing reports an open incident with failures whose
// announcement was decided and dropped by the scope.
func unheardFailing(p *Incident) bool {
	return p.Scope == ScopeOut && !p.Held && wasAnnounced(p) &&
		(p.State == Open || p.State == Flapping) &&
		p.Tier > Silent && len(p.Members) > 0
}

// AnnounceUnheard decides the announcement of incident id again, now
// that it is in scope: the first message people get about it. It
// reports false when the incident is gone, over, or was heard after
// all. Reminders count from this announcement.
func (m *Manager) AnnounceUnheard(id string, now time.Time) (Decision, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.incidents[id]
	if p == nil || !unheardFailing(p) {
		return Decision{}, false
	}
	p.Scope, p.AnnouncedRoute = ScopeIn, nil
	m.noteScopeWaiting(id, false)
	p.Announced, p.Reminded, p.Reminders = now, time.Time{}, 0
	p.note(now, "back in scope: announced")
	logLifecycle(p, "scope", "incident back in scope: announced")
	d := m.decide(p, Announce, ReasonSettled)
	m.relateAmong(&d.Incident, m.indexLive())
	return d, true
}

// RecordScopeWaiting keeps the announcement of incident id unheard
// while the scope has no verdict on it yet (a Namespace whose labels
// are not known). Like an announcement out of scope, it reached nobody,
// so Unheard lists it and the scope is asked again; the caller does not
// count it as withheld.
func (m *Manager) RecordScopeWaiting(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.incidents[id]; p != nil {
		p.Scope = ScopeOut
		p.sent.withdraw()
		m.noteScopeWaiting(id, true)
	}
}
