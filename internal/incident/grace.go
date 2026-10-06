package incident

import "time"

// awaitingEvidence reports whether an incident without members must keep
// its state: during the restore grace detectors may not have re-raised
// its findings yet, and a root kind kwatch cannot observe says nothing
// about recovery.
func (m *Manager) awaitingEvidence(p *Incident, now time.Time) bool {
	if len(p.Members) > 0 {
		return false
	}
	return m.inGrace(p, now) || !m.verifiable(p)
}

// inGrace reports whether p is a restored incident still inside the
// restore grace. Incidents opened after the restart have no stale
// model to wait for: their members come from this session's detectors.
func (m *Manager) inGrace(p *Incident, now time.Time) bool {
	return p.restored && now.Before(m.restoreGrace)
}

// holdRecovery reports whether a recovering incident without members
// must not resolve yet. During the restore grace the model is still being
// rebuilt, so missing members say nothing; the hold keeps running. A root
// kwatch cannot observe restarts the hold instead of resolving on missing
// data.
func (m *Manager) holdRecovery(p *Incident, now time.Time) bool {
	if !m.awaitingEvidence(p, now) {
		return false
	}
	if !m.verifiable(p) {
		p.RecoveringSince = now
	}
	return true
}

// afterGrace moves a deadline that falls inside the restore grace to the
// end of the grace, when recovery may first be decided.
func (m *Manager) afterGrace(
	p *Incident, at, now time.Time,
) time.Time {
	if m.inGrace(p, now) && at.Before(m.restoreGrace) {
		return m.restoreGrace
	}
	return at
}

func (m *Manager) verifiable(p *Incident) bool {
	return m.cfg.Verifiable == nil || m.cfg.Verifiable(p.Root.Kind)
}
