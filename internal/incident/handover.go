package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
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
	if m.cfg.NoHandover || !p.restored || len(p.Members) > 0 ||
		p.SupersededBy != "" || !wasAnnounced(p) || m.inGrace(p, now) {
		return
	}
	target := m.holderOf(p)
	if target == nil {
		return
	}
	p.SupersededBy, p.SupersededRoot = target.ID, target.Root
	p.note(now, "cause revised: its failures moved to "+describe(target.Root))
}

// holderOf finds the announced incident, other than p, with a failing
// member that is p's root or belongs to the same workload. Ties go to the
// lowest ID, so the choice does not depend on map order. One still
// settling is never the holder: the closed thread says "Moved" and points
// at the holder's message, which a settling incident may never send (it
// recovers unheard, or its tier is digest or silent). p waits instead and
// is handed over once the holder is announced, or goes on by itself.
func (m *Manager) holderOf(p *Incident) *Incident {
	for _, id := range m.sortedIDs() {
		q := m.incidents[id]
		if q != p && wasAnnounced(q) && m.failsFor(q, p) {
			return q
		}
	}
	return nil
}

// failsFor reports whether q has a failure of p's root or of its
// workload, or holds the stuck deletions that p was about.
func (m *Manager) failsFor(q, p *Incident) bool {
	root := p.Root
	if sameStuckStory(q, p) {
		return true
	}
	for key, f := range q.Members {
		if f.Advisory {
			continue
		}
		if key.Entity == root || m.wasMember(p, key.Entity) ||
			(m.model != nil && sameChainBehind(m.model, root, key.Entity)) {
			return true
		}
	}
	return false
}

// sameStuckStory reports that q is the incident of a finalizer that holds
// the objects whose deletion p, rooted at a workload, was about: the
// workload was once blamed for a controller that does not run, and now
// the finalizer is. The objects are not part of the workload's chain, so
// the mode and the namespace tell it.
func sameStuckStory(q, p *Incident) bool {
	return p.Mode == detection.ModeStuckDeleting &&
		q.Root.Kind == explain.KindFinalizer &&
		q.Root.Namespace == p.Root.Namespace &&
		q.Mode == detection.ModeStuckDeleting
}

// wasMember reports whether entity, or the pod it belongs to, failed in p
// when p was saved. The root may not tell: a virtual or deleted root
// (a Service that does not exist, say) is not recreated after a restart,
// so the failures that were p's come back under another root while p
// gets no member again. Those failures still go on, so p is handed over
// to the incident that holds one of them, never resolved as healthy.
func (m *Manager) wasMember(p *Incident, entity inventory.EntityID) bool {
	if len(p.formerMembers) == 0 {
		return false
	}
	if slices.Contains(p.formerMembers, entity) {
		return true
	}
	if m.model == nil {
		return false
	}
	pod, ok := rootcause.PodOf(m.model, entity)
	return ok && slices.Contains(p.formerMembers, pod)
}
