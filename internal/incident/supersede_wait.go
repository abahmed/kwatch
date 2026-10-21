package incident

import "time"

// takeoverDue is when the incident that took p's failures over speaks,
// while it still owes the update that says its cause was revised. The
// update waits ReviseSettle so what joins the new cause is in it, so the
// closes of the incidents it replaced wait for it too: their threads say
// "Moved" and point at that message, which must exist when they do.
func (m *Manager) takeoverDue(p *Incident) (time.Time, bool) {
	t := m.incidents[p.SupersededBy]
	if t == nil || !t.Pending.RevisedOwed() {
		return time.Time{}, false
	}
	return t.Pending.RevisedDueAt(m.cfg.ReviseSettle), true
}

// takeoverPending reports a superseded incident whose close still waits
// for the update of the incident that took over.
func (m *Manager) takeoverPending(p *Incident, now time.Time) bool {
	due, ok := m.takeoverDue(p)
	return ok && now.Before(due)
}
