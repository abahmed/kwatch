package explain

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// StatefulSets and DaemonSets keep their history in ControllerRevisions
// instead of ReplicaSets. A rollback of one returns to the revision
// before its newest, by the number "kubectl rollout undo
// --to-revision" names, and the pods made from a revision carry its
// name (or, for a DaemonSet, the hash that ends it) in their
// controller-revision-hash label.

// revisionWorkloadOf finds the StatefulSet or DaemonSet an entity is or
// whose pod it runs in.
func revisionWorkloadOf(
	model inventory.Reader, id inventory.EntityID,
) (inventory.EntityID, bool) {
	if revisionKind(id.Kind) {
		return id, true
	}
	pod, ok := rootcause.PodOf(model, id)
	if !ok {
		return inventory.EntityID{}, false
	}
	chain := rootcause.OwnerChain(model, pod)
	if len(chain) == 0 || !revisionKind(chain[0].Kind) {
		return inventory.EntityID{}, false
	}
	return chain[0], true
}

func revisionKind(kind inventory.Kind) bool {
	return kind == kube.KindStatefulSet || kind == kube.KindDaemonSet
}

// controllerPrevious returns the ControllerRevision with the highest
// revision below the workload's newest, and that revision. The revision
// is empty when there is none.
func controllerPrevious(
	model inventory.Reader, workload inventory.EntityID,
) (inventory.EntityID, string) {
	newest, previous := 0, 0
	var newestRev, previousRev inventory.EntityID
	for _, cr := range model.Related(
		workload, inventory.OwnedBy, inventory.Incoming,
	) {
		if cr.Kind != kube.KindControllerRevision {
			continue
		}
		revision, ok := revisionOf(model, cr)
		switch {
		case !ok:
		case revision > newest:
			previous, previousRev = newest, newestRev
			newest, newestRev = revision, cr
		case revision > previous && revision < newest:
			previous, previousRev = revision, cr
		}
	}
	if previous == 0 {
		return inventory.EntityID{}, ""
	}
	return previousRev, strconv.Itoa(previous)
}

// controllerNewest returns the workload's ControllerRevision with the
// highest revision: the one its pods are being brought to.
func controllerNewest(
	model inventory.Reader, workload inventory.EntityID,
) (inventory.EntityID, bool) {
	newest := 0
	var newestRev inventory.EntityID
	for _, cr := range model.Related(
		workload, inventory.OwnedBy, inventory.Incoming,
	) {
		if cr.Kind != kube.KindControllerRevision {
			continue
		}
		if revision, ok := revisionOf(model, cr); ok && revision > newest {
			newest, newestRev = revision, cr
		}
	}
	return newestRev, newest > 0
}

// revisionServed reports positive evidence that the ControllerRevision
// cr of workload served: a pod of the workload made from it is ready.
// A revision whose pods are all gone leaves no such record.
func revisionServed(
	model inventory.Reader, workload, cr inventory.EntityID,
) bool {
	for _, pod := range rootcause.OwnedPods(model, workload) {
		if podOfRevision(model, pod, workload, cr) && podReady(model, pod) {
			return true
		}
	}
	return false
}

// podOfRevision reports a pod whose controller-revision-hash label
// names cr: the revision's whole name (StatefulSet) or the hash after
// the workload's name (DaemonSet).
func podOfRevision(
	model inventory.Reader, pod, workload, cr inventory.EntityID,
) bool {
	hash := kube.PodRevisionHash(model, pod)
	return hash != "" &&
		(hash == cr.Name || workload.Name+"-"+hash == cr.Name)
}

func podReady(model inventory.Reader, pod inventory.EntityID) bool {
	a, ok := model.Attribute(pod, kube.AttrReady)
	if !ok {
		return false
	}
	ready, _ := a.Value.AsBool()
	return ready
}
