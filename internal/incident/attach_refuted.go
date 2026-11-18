package incident

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// A cause that is a change of the root itself (an edit, a rollout, the
// object's creation) is dropped when the solver refutes that change
// (explain.Area.ChangeRefuted: a workload held at zero replicas by a
// change that did not set them, a change its controller has not even
// read), no failure of the area is blamed on a cause any more, and
// every failure of the incident was in that solve. A cause whose
// evidence merely aged out is never dropped this way: the thread keeps
// its words (TestWithdrawnCauseSendsNoRevision).

// ownChangeSolves records, per root an area placed failures on, whether
// any of those placements carried a cause.
type ownChangeSolves map[inventory.EntityID]bool

// note records one placement.
func (o ownChangeSolves) note(where placement) {
	o[where.root] = o[where.root] || where.cause != nil
}

// dropRefutedChanges drops the own-change cause of every incident whose
// root's change the solve refuted and placed without a cause, once
// all its failing members were placed by the same solve.
func (m *Manager) dropRefutedChanges(refuted []inventory.EntityID,
	solved ownChangeSolves, placed map[detection.Key]bool,
) {
	for _, root := range refuted {
		withCause, seen := solved[root]
		p := m.lookup(root)
		if !seen || withCause || p == nil || p.State == Resolved ||
			!ownChange(p) || !allPlaced(p, placed) {
			continue
		}
		p.Cause = nil
		p.Pending.reshaped = true
		m.changed[p.ID] = true
	}
}

// ownChange reports a cause that is a change of the incident's root.
func ownChange(p *Incident) bool {
	return p.Cause != nil && p.Cause.Change != nil && p.Cause.Root == p.Root
}

// allPlaced reports whether every failing member of p was placed.
func allPlaced(p *Incident, placed map[detection.Key]bool) bool {
	for key, f := range p.Members {
		if !f.Advisory && !placed[key] {
			return false
		}
	}
	return true
}
