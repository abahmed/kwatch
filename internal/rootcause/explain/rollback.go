package explain

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// rollbackRevision is the Deployment revision a rollback of a blamed
// rollout returns to: the one before the Deployment's newest
// ReplicaSet, whichever revision the covered pod itself runs. Covers
// are tried in order until one belongs to a Deployment with an earlier
// revision; it is empty when none does.
func rollbackRevision(
	model inventory.Reader, covers []inventory.EntityID,
) string {
	for _, id := range covers {
		deployment, ok := deploymentOf(model, id)
		if !ok {
			continue
		}
		if revision := deploymentPrevious(model, deployment); revision != "" {
			return revision
		}
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

// deploymentPrevious returns the highest ReplicaSet revision below the
// newest one of a Deployment: the revision a rollback returns to when
// the finding names the Deployment itself, not one of its pods.
func deploymentPrevious(
	model inventory.Reader, deployment inventory.EntityID,
) string {
	newest, previous := 0, 0
	for _, rs := range model.Related(
		deployment, inventory.OwnedBy, inventory.Incoming,
	) {
		revision, ok := revisionOf(model, rs)
		switch {
		case !ok:
		case revision > newest:
			previous, newest = newest, revision
		case revision > previous && revision < newest:
			previous = revision
		}
	}
	if previous == 0 {
		return ""
	}
	return strconv.Itoa(previous)
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
