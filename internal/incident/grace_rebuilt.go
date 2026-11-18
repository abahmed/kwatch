package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// The restore grace ends when the model a restored incident is judged
// from is rebuilt, not after a fixed time. The engine reports when the
// required sources synced plus a settle (SettleGrace): one full
// detection pass and the first kubelet-stats and log rounds. From then
// on each restored incident leaves the grace on its own once every kind
// it reads is synced (an optional kind syncs later) and no detector is
// still counting a wait on one of its former objects (a sustain counted
// from when kwatch first saw the bad state re-raises only after it).
// restoreGrace stays the upper bound: a sync that never completes, or a
// kind that never syncs, ends the grace there.

// graceRecheck is how often an incident still waiting for its model
// looks again: a kind syncing or a wait ending changes no incident.
const graceRecheck = 15 * time.Second

// SettleGrace records when the model is rebuilt: the required sources
// synced and a settle passed. Before it, restored incidents keep the
// whole grace. Only the first call counts; without a restored live
// incident there is no grace to settle.
func (m *Manager) SettleGrace(at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.restoreGrace.IsZero() || !m.graceSettled.IsZero() {
		return
	}
	m.graceSettled = at
}

// Rebuilding reports whether a restored live incident is still inside
// the restore grace at now.
func (m *Manager) Rebuilding(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.restoreGrace.IsZero() || !now.Before(m.restoreGrace) {
		return false
	}
	for _, p := range m.incidents {
		if p.State != Resolved && m.inGrace(p, now) {
			return true
		}
	}
	return false
}

// rebuilt reports whether the model p is judged from is rebuilt: the
// sources synced a settle ago, every kind p reads is synced and no
// detector still waits out a bad state on a former object of p.
func (m *Manager) rebuilt(p *Incident, now time.Time) bool {
	if m.graceSettled.IsZero() || now.Before(m.graceSettled) {
		return false
	}
	for _, id := range m.graceObjects(p) {
		if m.cfg.Verifiable != nil && !m.cfg.Verifiable(id.Kind) {
			return false
		}
		if hasMemberOn(p, id) {
			continue
		}
		if m.cfg.Watching != nil && m.cfg.Watching(id) {
			return false
		}
	}
	return !ownsPods(p.Root.Kind) || m.cfg.Verifiable == nil ||
		m.cfg.Verifiable(kube.KindPod)
}

// graceObjects are the objects whose findings p waits for: its root,
// its former members and its current ones.
func (m *Manager) graceObjects(p *Incident) []inventory.EntityID {
	out := make([]inventory.EntityID, 0,
		1+len(p.formerMembers)+len(p.Members))
	out = append(out, p.Root)
	out = append(out, p.formerMembers...)
	for key := range p.Members {
		out = append(out, key.Entity)
	}
	return out
}

// hasMemberOn reports whether id has a failing member in p: its
// finding is back, so a wait counted on it is that finding's own.
func hasMemberOn(p *Incident, id inventory.EntityID) bool {
	for key := range p.Members {
		if key.Entity == id {
			return true
		}
	}
	return false
}

// graceEnd is when p is next judged out of the grace: the end of the
// settle, the next look of an incident still waiting for its model, and
// never later than the upper bound.
func (m *Manager) graceEnd(p *Incident, now time.Time) time.Time {
	switch {
	case m.graceSettled.IsZero():
		return m.restoreGrace
	case now.Before(m.graceSettled):
		return earliest(m.graceSettled, m.restoreGrace)
	case !m.rebuilt(p, now):
		return earliest(now.Add(graceRecheck), m.restoreGrace)
	}
	return now
}

// rebuildFloor is when the model as a whole was rebuilt: annotations
// first seen before it were listed with the model, not set since.
func (m *Manager) rebuildFloor() time.Time {
	if m.graceSettled.IsZero() {
		return m.restoreGrace
	}
	return earliest(m.graceSettled, m.restoreGrace)
}
