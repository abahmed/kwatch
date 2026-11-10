package incident

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// childKinds are the objects a controller root creates and deletes by
// itself: the Jobs a CronJob starts on schedule, the ReplicaSets of a
// Deployment's rollouts. (A Pod's records are never a fix: notAFix.)
var childKinds = map[inventory.Kind]inventory.Kind{
	kube.KindCronJob:    kube.KindJob,
	kube.KindDeployment: kube.KindReplicaSet,
}

// notAFixOf is notAFix for an incident on root: it also skips the
// creation or deletion of root's own controller-made children. The next
// scheduled Job that runs fine is the CronJob doing its work, not
// someone's fix; a change to the root itself still counts.
func notAFixOf(
	model inventory.Reader, root inventory.EntityID,
) func(inventory.Change) bool {
	return func(change inventory.Change) bool {
		return notAFix(change) || controllerChild(model, root, change)
	}
}

// controllerChild reports a change that creates or deletes a child of
// root of the kind root's controller makes. A child still in the model
// must be owned by root; a deleted one is known by the name the
// controller gives it, root's name and a suffix.
func controllerChild(
	model inventory.Reader, root inventory.EntityID, change inventory.Change,
) bool {
	id := change.Entity
	kind, ok := childKinds[root.Kind]
	if !ok || id.Kind != kind || id.Namespace != root.Namespace ||
		!(change.Created || change.Deleted) {
		return false
	}
	if model != nil && model.Exists(id) {
		owner, owned := inventory.Owner(model, id)
		return owned && owner == root
	}
	return strings.HasPrefix(id.Name, root.Name+"-")
}
