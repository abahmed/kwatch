package explain

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// rollbackRevision is the Deployment revision a rollback of a blamed
// rollout returns to, read from the first failing pod the cause covers.
// It is empty when no covered pod runs under a Deployment with known
// revisions.
func rollbackRevision(
	model inventory.Reader, covers []inventory.EntityID,
) string {
	for _, id := range covers {
		if pod, ok := rootcause.PodOf(model, id); ok {
			return previousRevision(model, pod)
		}
	}
	return ""
}

// previousRevision returns the Deployment revision before the one the
// pod runs: the highest ReplicaSet revision below the pod's own.
func previousRevision(
	model inventory.Reader, pod inventory.EntityID,
) string {
	chain := ownerChain(model, pod)
	if len(chain) < 2 || chain[0].Kind != kube.KindReplicaSet ||
		chain[1].Kind != kube.KindDeployment {
		return ""
	}
	current, ok := revisionOf(model, chain[0])
	if !ok {
		return ""
	}
	best := 0
	for _, rs := range model.Related(
		chain[1], inventory.OwnedBy, inventory.Incoming,
	) {
		if revision, ok := revisionOf(model, rs); ok &&
			revision < current && revision > best {
			best = revision
		}
	}
	if best == 0 {
		return ""
	}
	return strconv.Itoa(best)
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
