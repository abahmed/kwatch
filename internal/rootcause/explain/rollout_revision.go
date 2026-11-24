package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// notRollout reports a change of workload id that rolled out nothing
// effect runs, so it cannot explain effect through the workload's own
// pods (LinkOwns). A creation always rolls out. An edit that leaves the
// pod template alone (only the replica count or a pause) starts no new
// revision. And a pod whose ReplicaSet neither was created nor became
// the current revision since the change runs a revision older than it:
// a pod restarted or rescheduled onto new nodes (a cold start) is not
// the change's doing. When the pod's revision cannot be read, the
// change is kept.
func (v *view) notRollout(
	id, effect inventory.EntityID, change inventory.Change,
) bool {
	if change.Created {
		return false
	}
	if !templateEdit(change) {
		return true
	}
	revision, ok := v.podRevision(id, effect)
	if !ok {
		return false
	}
	from := change.At.Add(-TemporalSlack)
	if created, ok := attrTime(revision, kube.AttrCreated); !ok ||
		!created.Before(from) {
		return false
	}
	current, ok := revision.Attribute(kube.AttrRevision)
	return ok && current.Since.Before(from)
}

// templateEdit reports a change that edits more than the replica count
// and the pause of a workload: a change of its pod template. A change
// with no fields recorded is kept as one.
func templateEdit(change inventory.Change) bool {
	if len(change.Fields) == 0 {
		return true
	}
	for _, field := range change.Fields {
		if field.Path != "spec.replicas" && field.Path != "spec.paused" {
			return true
		}
	}
	return false
}

// podRevision is the ReplicaSet of workload id that runs effect's pod.
func (v *view) podRevision(
	id, effect inventory.EntityID,
) (inventory.Entity, bool) {
	pod, ok := v.unitOf(effect)
	if !ok || id.Kind != kube.KindDeployment {
		return inventory.Entity{}, false
	}
	for _, rs := range v.s.Model.Related(pod, inventory.OwnedBy,
		inventory.Outgoing) {
		if rs.Kind != kube.KindReplicaSet {
			continue
		}
		for _, owner := range v.s.Model.Related(rs, inventory.OwnedBy,
			inventory.Outgoing) {
			if owner == id {
				return v.s.Model.Entity(rs)
			}
		}
	}
	return inventory.Entity{}, false
}
