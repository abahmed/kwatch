package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// An incident whose cause was revised moves to a new root: a crash first
// blamed on a ConfigMap is later explained by its Deployment. When the
// failure returns after the incident resolved, the reasoning may name the
// first root again. Found by root alone, the return would be a new
// incident with no history, and whether people heard "failing again" or
// a new story would depend on that internal revision. So an incident
// remembers every root it left, and a failure at one of them finds it:
// within RepageWindow of the resolve the incident reopens, after it the
// new incident is linked to it as its previous one.

// maxPastRoots bounds the roots an incident remembers; the latest win.
const maxPastRoots = 4

// notePastRoot remembers that p left root.
func (m *Manager) notePastRoot(p *Incident, root inventory.EntityID) {
	p.pastRoots = slices.DeleteFunc(p.pastRoots,
		func(id inventory.EntityID) bool { return id == root })
	p.pastRoots = append(p.pastRoots, root)
	if over := len(p.pastRoots) - maxPastRoots; over > 0 {
		p.pastRoots = slices.Clone(p.pastRoots[over:])
	}
	m.indexPastRoots(p)
}

// indexPastRoots makes p the incident each of its past roots leads to.
func (m *Manager) indexPastRoots(p *Incident) {
	if m.pastRoots == nil {
		m.pastRoots = map[string]string{}
	}
	for _, root := range p.pastRoots {
		m.pastRoots[root.String()] = p.ID
	}
}

// restorePastRoots rebuilds the past roots of a restored incident. A
// record from before they were saved has its former root alone.
func (m *Manager) restorePastRoots(p *Incident) {
	if len(p.pastRoots) == 0 && p.formerRoot != (inventory.EntityID{}) {
		p.pastRoots = []inventory.EntityID{p.formerRoot}
	}
	m.indexPastRoots(p)
}

// carryRoots gives a recurrence the roots of the incident it replaces:
// its past roots and its root, except the recurrence's own root.
func carryRoots(p, previous *Incident) {
	roots := append(slices.Clone(previous.pastRoots), previous.Root)
	roots = slices.DeleteFunc(roots,
		func(id inventory.EntityID) bool { return id == p.Root })
	p.pastRoots = roots[max(len(roots)-maxPastRoots, 0):]
}

// pastRootIncident is the resolved incident that left root, or nil.
func (m *Manager) pastRootIncident(root inventory.EntityID) *Incident {
	p := m.incidents[m.pastRoots[root.String()]]
	if p == nil || p.State != Resolved || p.Root == root ||
		!slices.Contains(p.pastRoots, root) {
		return nil
	}
	return p
}

// returnsHere reopens previous, an incident that left root, for a
// failure at root less than RepageWindow after it resolved: the outage
// is back, whatever root it was revised to. It moves back to root, where
// the reasoning places the failure now, and reports whether it reopened.
func (m *Manager) returnsHere(
	now time.Time, previous *Incident, s detection.Finding,
	root inventory.EntityID,
) bool {
	if previous == nil || !reopenable(previous, now, s.Mode) ||
		m.reopenRefused(now, previous, s, root) {
		return false
	}
	// Reopen first: it drops the resolve filed under the old root.
	m.reopen(previous, now)
	m.unindex(previous)
	m.notePastRoot(previous, previous.Root)
	previous.Root = root
	m.index(previous)
	return true
}

// prunePastRoots drops the entries whose incident is gone or no longer
// remembers the root.
func (m *Manager) prunePastRoots() {
	for root, id := range m.pastRoots {
		p := m.incidents[id]
		if p == nil || !slices.ContainsFunc(p.pastRoots,
			func(r inventory.EntityID) bool { return r.String() == root }) {
			delete(m.pastRoots, root)
		}
	}
}
