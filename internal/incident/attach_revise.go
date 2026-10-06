package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Cause revision: when a finding's cause changes, it leaves its
// incident for the incident of the new root, or the whole incident moves.

// beganTooLate reports whether cause began more than
// explain.TemporalExclusion after the announced incident id was opened.
// A cause with an unknown start, or an incident nobody heard of yet,
// never counts: the first message may still name the better cause.
func (m *Manager) beganTooLate(
	id string, cause *rootcause.CauseRecord,
) bool {
	p := m.incidents[id]
	if p == nil || cause == nil || cause.Began.IsZero() ||
		!wasAnnounced(p) || p.Opened.IsZero() {
		return false
	}
	return cause.Began.After(p.Opened.Add(explain.TemporalExclusion))
}

// holds reports whether incident id is the current incident of root.
func (m *Manager) holds(id string, root inventory.EntityID) bool {
	p := m.lookup(root)
	return p != nil && p.ID == id
}

// revise moves s away from incident previous after its cause changed to
// root. When that leaves an announced incident empty, the incident is
// either superseded, because root already has its own announced
// incident, or moved to root so the conversation continues under the
// same ID. It is moved only when every member that left since people
// last heard about it went to root: that is the same story with a
// revised cause. Members that dispersed to several roots, as when a
// node pool stops failing as a whole and its workloads fail on their
// own again, end the story instead; the empty incident recovers and
// resolves on its own, and the workloads are announced anew. It reports
// whether the incident was moved.
func (m *Manager) revise(
	now time.Time, s detection.Finding, previous string,
	root inventory.EntityID,
) bool {
	key := s.Key()
	delete(m.byMember, key)
	old := m.incidents[previous]
	if old == nil {
		return false
	}
	delete(old.Members, key)
	logMember("moved out of", old, key)
	m.changed[old.ID] = true
	old.noteMoved(root)
	if !hasFailingMember(old) {
		// Only configuration risks are left: they go with the failure.
		m.dropAdvisories(old)
	}
	old.note(now, "cause revised: "+describe(s.Entity)+
		" is now explained by "+describe(root))
	if len(old.Members) > 0 || !wasAnnounced(old) {
		return false
	}
	if target := m.lookup(root); target != nil && wasAnnounced(target) {
		old.SupersededBy, old.SupersededRoot = target.ID, target.Root
		return false
	}
	if dispersed(old.movedTo) {
		old.note(now, "its failures went their own ways; this ends here")
		return false
	}
	m.reroot(now, old, root)
	return true
}

// maxMoved bounds movedTo. Two different roots already mean "dispersed",
// so a few more say nothing new; a flapping incident whose members swap
// cause again and again would otherwise grow the list for as long as it
// flaps.
const maxMoved = 4

// noteMoved records that a member left for root: once per root, and
// never past maxMoved roots.
func (p *Incident) noteMoved(root inventory.EntityID) {
	if len(p.movedTo) < maxMoved && !slices.Contains(p.movedTo, root) {
		p.movedTo = append(p.movedTo, root)
	}
}

// dispersed reports whether the members left for more than one root.
func dispersed(roots []inventory.EntityID) bool {
	for _, root := range roots {
		if root != roots[0] {
			return true
		}
	}
	return false
}

// reroot moves incident p to root and marks its next update as a cause
// revision. An incident of root that is still settling is merged into p:
// nobody has heard of it yet.
func (m *Manager) reroot(
	now time.Time, p *Incident, root inventory.EntityID,
) {
	// The old root's cause does not explain the new root: the members
	// that move here bring the new one.
	p.Cause, p.CauseUnclear = nil, false
	if target := m.lookup(root); target != nil && target.State == Settling {
		for key, s := range target.Members {
			p.Members[key] = s
			m.byMember[key] = p.ID
		}
		p.Cause, p.CauseUnclear = target.Cause, target.CauseUnclear
		p.Unverified = rootcause.MergeUnverified(
			p.Unverified, target.Unverified)
		delete(m.incidents, target.ID)
	}
	m.unindex(p)
	p.Root = root
	m.index(p)
	p.movedTo = nil
	p.Pending.MarkRevised(now)
	p.note(now, "cause revised: now explained by "+describe(root))
}
