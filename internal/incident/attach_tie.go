package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// edgesOutIncumbent reports a new cause that only ties with the root of
// the announced incident the finding already belongs to. The first
// minutes of an incident are when the picture forms and a better cause
// may still replace the first one (explain.TemporalExclusion); after
// that, a solver that finds the old root a rival within the margin, or
// one that explains the failures as well or better through the same
// row, has only re-ranked. A re-rank is not a new cause: the finding
// stays, so a peer that edges ahead every few hours does not move the
// incident, re-announce it or page again.
func (m *Manager) edgesOutIncumbent(
	now time.Time, previous string, where placement,
) bool {
	p := m.incidents[previous]
	if p == nil || !wasAnnounced(p) || where.cause == nil ||
		now.Sub(p.Announced) <= explain.TemporalExclusion {
		return false
	}
	if rival := where.cause.Rival; rival != nil && p.statesRoot(rival.Root) {
		return true
	}
	return slices.ContainsFunc(where.outscored, p.statesRoot)
}

// statesRoot reports whether root is what the incident is rooted at, or
// the root of the cause it states.
func (inc *Incident) statesRoot(root inventory.EntityID) bool {
	return root == inc.Root || (inc.Cause != nil && inc.Cause.Root == root)
}
