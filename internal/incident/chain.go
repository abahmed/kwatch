package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Cause-revision damping. When one workload fails and recovers, the
// reasoning may blame its pod, then its Service, then its Deployment,
// each time for the very same failures. Telling people "cause revised"
// for each flip is noise: nothing changed but the name of the culprit.

// failingKeys is the set of failure findings of p, used to tell whether
// a later decision is about the same failures.
func failingKeys(p *Incident) map[detection.Key]struct{} {
	keys := make(map[detection.Key]struct{}, len(p.Members))
	for key, member := range p.Members {
		if !member.Advisory {
			keys[key] = struct{}{}
		}
	}
	return keys
}

// sameChainFlip reports that finding key, which belongs to the announced
// incident previous, is only being re-blamed on another object of the
// same workload chain: it was one of the failures people were last told
// about, and the new root is the workload, its Service, its pods or
// ReplicaSets. Such a finding stays where it is, so the root stays as
// first announced.
func (m *Manager) sameChainFlip(
	previous string, key detection.Key, root inventory.EntityID,
) bool {
	p := m.incidents[previous]
	if p == nil || !wasAnnounced(p) || m.model == nil {
		return false
	}
	if _, told := p.decided[key]; !told {
		return false
	}
	return sameChain(m.model, p.Root, root)
}

// sameChain reports whether a and b lead to a common workload.
func sameChain(model inventory.Reader, a, b inventory.EntityID) bool {
	mine := workloadsOf(model, a)
	for _, id := range workloadsOf(model, b) {
		for _, other := range mine {
			if id == other {
				return true
			}
		}
	}
	return false
}

// workloadsOf names the workloads an entity is part of: a pod or a
// ReplicaSet belongs to its top owner, a Service to the owners of the
// pods behind it.
func workloadsOf(
	model inventory.Reader, id inventory.EntityID,
) []inventory.EntityID {
	if id.Kind != kube.KindService {
		return []inventory.EntityID{defaultRoot(model, id)}
	}
	var out []inventory.EntityID
	for _, slice := range model.Related(
		id, inventory.Backs, inventory.Incoming,
	) {
		for _, pod := range model.Related(
			slice, inventory.RoutesTo, inventory.Outgoing,
		) {
			out = append(out, rootcause.TopOwner(model, pod))
		}
	}
	return out
}

// sameFailures reports whether every failure of target is one that
// people were last told about for p, and no other.
func sameFailures(p, target *Incident) bool {
	if len(p.decided) == 0 {
		return false
	}
	mine := failingKeys(target)
	if len(mine) != len(p.decided) {
		return false
	}
	for key := range mine {
		if _, ok := p.decided[key]; !ok {
			return false
		}
	}
	return true
}

// quietSupersede reports a superseded incident whose resolve is not
// worth sending: its failures went to an incident created within
// ReviseSettle and they are exactly the ones people were told about,
// so it is the same story under another root. A page, held or not, keeps
// its resolve, which is what closes its alert.
func (m *Manager) quietSupersede(p *Incident, now time.Time) bool {
	target := m.incidents[p.SupersededBy]
	return target != nil && !reachedPaging(p) &&
		now.Sub(target.Opened) <= m.cfg.ReviseSettle &&
		sameFailures(p, target)
}

// explanationLapsed reports that a finding of the announced incident
// previous would now move to a root with no cause only because the
// evidence that tied it to previous aged out, such as an event leaving
// its window, while the root of previous still fails. It stays: nothing
// about the failure changed. When the root itself is gone, as when a
// node pool stops failing as a whole, the findings do disperse.
func (m *Manager) explanationLapsed(
	previous string, where placement,
) bool {
	p := m.incidents[previous]
	if p == nil || !wasAnnounced(p) || where.cause != nil {
		return false
	}
	for member, s := range p.Members {
		if member.Entity == p.Root && !s.Symptom && !s.Advisory {
			return true
		}
	}
	return false
}

// inChain reports that p's cause blames, without a change, an object of
// the root's own workload chain other than the root itself.
func (m *Manager) inChain(p *Incident) bool {
	c := p.Cause
	return c != nil && c.Root != p.Root && c.Change == nil &&
		m.model != nil && sameChain(m.model, p.Root, c.Root)
}
