package explain

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// rollbackRevision is the revision a rollback of a blamed rollout
// returns to: for a Deployment the one before its newest ReplicaSet,
// for a StatefulSet or DaemonSet the one before its newest
// ControllerRevision, whichever revision a covered pod itself runs.
// workload is the one whose change is blamed (blamedWorkload), the one
// the undo names; it is empty when that workload has no earlier
// revision.
func rollbackRevision(
	model inventory.Reader, workload inventory.EntityID,
) string {
	return previousRevision(model, workload)
}

// blamedWorkload is the Deployment, StatefulSet or DaemonSet whose
// change is blamed, when a covered entity is or runs under it. Only
// that workload's revisions say what undoing the change returns to;
// another covered workload's say nothing about it. ok is false when no
// cover belongs to the changed workload, as for a change of a Secret.
func blamedWorkload(
	model inventory.Reader, change inventory.Change,
	covers []inventory.EntityID,
) (inventory.EntityID, bool) {
	for _, id := range covers {
		if w, ok := rolloutWorkloadOf(model, id); ok &&
			w == change.Entity {
			return w, true
		}
	}
	return inventory.EntityID{}, false
}

// rolloutWorkloadOf is the Deployment, StatefulSet or DaemonSet an
// entity is or runs under.
func rolloutWorkloadOf(
	model inventory.Reader, id inventory.EntityID,
) (inventory.EntityID, bool) {
	if deployment, ok := deploymentOf(model, id); ok {
		return deployment, true
	}
	return revisionWorkloadOf(model, id)
}

// previousRevision is the revision a rollback of the workload a
// covered entity belongs to returns to, or "" when there is none.
func previousRevision(
	model inventory.Reader, id inventory.EntityID,
) string {
	if deployment, ok := deploymentOf(model, id); ok {
		_, revision := deploymentPrevious(model, deployment)
		return revision
	}
	if workload, ok := revisionWorkloadOf(model, id); ok {
		_, revision := controllerPrevious(model, workload)
		return revision
	}
	return ""
}

// deploymentOf finds the Deployment an entity is or runs under, through
// its ReplicaSet.
func deploymentOf(
	model inventory.Reader, id inventory.EntityID,
) (inventory.EntityID, bool) {
	if id.Kind == kube.KindDeployment {
		return id, true
	}
	pod, ok := rootcause.PodOf(model, id)
	if !ok {
		return inventory.EntityID{}, false
	}
	chain := rootcause.OwnerChain(model, pod)
	if len(chain) < 2 || chain[0].Kind != kube.KindReplicaSet ||
		chain[1].Kind != kube.KindDeployment {
		return inventory.EntityID{}, false
	}
	return chain[1], true
}

// deploymentPrevious returns the ReplicaSet with the highest revision
// below the newest one of a Deployment, and that revision: the one a
// rollback returns to when the finding names the Deployment itself, not
// one of its pods. The revision is empty when there is none.
func deploymentPrevious(
	model inventory.Reader, deployment inventory.EntityID,
) (inventory.EntityID, string) {
	newest, previous := 0, 0
	var newestSet, previousSet inventory.EntityID
	for _, rs := range model.Related(
		deployment, inventory.OwnedBy, inventory.Incoming,
	) {
		revision, ok := revisionOf(model, rs)
		switch {
		case !ok:
		case revision > newest:
			previous, previousSet = newest, newestSet
			newest, newestSet = revision, rs
		case revision > previous && revision < newest:
			previous, previousSet = revision, rs
		}
	}
	if previous == 0 {
		return inventory.EntityID{}, ""
	}
	return previousSet, strconv.Itoa(previous)
}

// revisionOf reads a ReplicaSet's Deployment revision.
func revisionOf(model inventory.Reader, rs inventory.EntityID) (int, bool) {
	entity, ok := model.Entity(rs)
	if !ok {
		return 0, false
	}
	revision, err := strconv.Atoi(
		attributeValue(entity, kube.AttrRevision).AsText())
	return revision, err == nil && revision > 0
}
