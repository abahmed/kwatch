package explain

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// previousFails reports a blamed release of a Deployment whose previous
// version fails too: a pod of a ReplicaSet that existed before the
// release, and is not the Deployment's newest, has a container failing
// on its own. The release cannot be what broke it, and undoing it would
// bring back a version that fails as well. A change of another object
// (a Secret both versions read) says nothing here, nor does a
// ReplicaSet kwatch first saw after the release (a later fix attempt)
// or the newest one (a rollback to an earlier template).
func (s Snapshot) previousFails(
	change inventory.Change, covers []inventory.EntityID,
) bool {
	for _, id := range covers {
		deployment, ok := deploymentOf(s.Model, id)
		if !ok || change.Entity != deployment {
			continue
		}
		newest, _ := s.newestReplicaSet(deployment)
		for _, pod := range rootcause.OwnedPods(s.Model, deployment) {
			if s.onSetBefore(pod, newest, change.At) && s.podFailing(pod) {
				return true
			}
		}
	}
	return false
}

// onSetBefore reports a pod owned by a ReplicaSet other than newest
// that kwatch saw before t.
func (s Snapshot) onSetBefore(
	pod, newest inventory.EntityID, t time.Time,
) bool {
	for _, rs := range s.Model.Related(pod, inventory.OwnedBy,
		inventory.Outgoing) {
		e, ok := s.Model.Entity(rs)
		if ok && rs.Kind == kube.KindReplicaSet && rs != newest &&
			e.FirstSeen.Before(t) {
			return true
		}
	}
	return false
}

// newestReplicaSet is the ReplicaSet of a Deployment with the highest
// revision, or, when none states one, the one kwatch saw appear last;
// a tie names none.
func (s Snapshot) newestReplicaSet(
	deployment inventory.EntityID,
) (inventory.EntityID, bool) {
	sets := s.Model.Related(deployment, inventory.OwnedBy,
		inventory.Incoming)
	var newest inventory.EntityID
	best := 0
	for _, rs := range sets {
		if revision, ok := revisionOf(s.Model, rs); ok && revision > best {
			newest, best = rs, revision
		}
	}
	if best > 0 {
		return newest, true
	}
	return s.lastSeenSet(sets)
}

// lastSeenSet is the ReplicaSet first seen last, when one is strictly
// later than the others.
func (s Snapshot) lastSeenSet(
	sets []inventory.EntityID,
) (inventory.EntityID, bool) {
	var newest inventory.EntityID
	var at time.Time
	tie := false
	for _, rs := range sets {
		e, ok := s.Model.Entity(rs)
		switch {
		case !ok:
		case e.FirstSeen.After(at):
			newest, at, tie = rs, e.FirstSeen, false
		case e.FirstSeen.Equal(at):
			tie = true
		}
	}
	if at.IsZero() || tie {
		return inventory.EntityID{}, false
	}
	return newest, true
}

// podFailing reports a pod one of whose containers fails on its own: a
// crash loop or a failed start, not a pod that is only unready while it
// is replaced.
func (s Snapshot) podFailing(pod inventory.EntityID) bool {
	for _, id := range s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		for _, f := range s.Findings[id] {
			if f.Health == detection.Failing && !f.Advisory &&
				!f.Symptom {
				return true
			}
		}
	}
	return false
}
