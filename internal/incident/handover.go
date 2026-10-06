package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// handOver supersedes a restored incident whose failures live on in
// another incident. A restart keeps no members, and the findings of the
// restored root may come back under a different root: a Service that
// has no endpoints is explained by its crashing Deployment. The old
// incident then never gets a member again, and would end as "stable" or
// "healthy" while its failure goes on. It is closed as superseded by
// the incident that holds the failure now. It waits for the restore
// grace, so findings still coming back are not mistaken for gone ones.
func (m *Manager) handOver(p *Incident, now time.Time) {
	if !p.restored || len(p.Members) > 0 || p.SupersededBy != "" ||
		!wasAnnounced(p) || m.inGrace(p, now) {
		return
	}
	target := m.holderOf(p)
	if target == nil {
		return
	}
	p.SupersededBy, p.SupersededRoot = target.ID, target.Root
	p.note(now, "cause revised: its failures moved to "+describe(target.Root))
}

// holderOf finds the live incident, other than p, with a failing member
// that is p's root or belongs to the same workload. An announced
// incident is preferred over one still settling; ties go to the lowest
// ID, so the choice does not depend on map order.
func (m *Manager) holderOf(p *Incident) *Incident {
	var best *Incident
	for _, id := range m.sortedIDs() {
		q := m.incidents[id]
		if q == p || q.State == Resolved || !m.failsFor(q, p.Root) {
			continue
		}
		if wasAnnounced(q) {
			return q
		}
		if best == nil {
			best = q
		}
	}
	return best
}

// failsFor reports whether q has a failure of root or of its workload.
func (m *Manager) failsFor(q *Incident, root inventory.EntityID) bool {
	for key, f := range q.Members {
		if f.Advisory {
			continue
		}
		if key.Entity == root ||
			(m.model != nil && sameChain(m.model, root, key.Entity)) {
			return true
		}
	}
	return false
}
