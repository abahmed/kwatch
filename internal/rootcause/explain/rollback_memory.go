package explain

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A completed rollout scales the old ReplicaSet to zero and removes
// its pods (a StatefulSet or DaemonSet replaces every pod), so the live
// evidence that the revision a rollback returns to served is gone. The
// model remembers, per workload, the revisions kwatch saw serving
// (inventory.ServedRevision); ServingRevision says what to remember
// and previousServed reads the memory beside the live evidence.

// servedReader is the model's memory of the revisions each workload
// served. *inventory.Model implements it; a model without it remembers
// nothing.
type servedReader interface {
	ServedRevisions(
		workload inventory.EntityID, since time.Time,
	) []inventory.ServedRevision
}

// ServingRevision reports the workload an observed entity belongs to
// and the name of the workload's current revision when that revision
// serves now: for a Deployment its newest ReplicaSet has ready
// replicas, for a StatefulSet or DaemonSet a ready pod was made from
// its newest ControllerRevision. An observed pod speaks only for
// itself (ready and of the current revision), so a pod update costs
// one look however large the workload. ok is false for any other
// entity and while nothing of the current revision is ready. Only what
// is seen now counts, so the memory holds observations, never
// inferences.
func ServingRevision(
	model inventory.Reader, id inventory.EntityID,
) (workload inventory.EntityID, name string, ok bool) {
	workload, ok = servingWorkloadOf(model, id)
	if !ok {
		return inventory.EntityID{}, "", false
	}
	current, ok := currentRevision(model, workload)
	if !ok || !servingNow(model, id, workload, current) {
		return inventory.EntityID{}, "", false
	}
	return workload, current.Name, true
}

// currentRevision is the newest ReplicaSet of a Deployment or the
// newest ControllerRevision of a StatefulSet or DaemonSet.
func currentRevision(
	model inventory.Reader, workload inventory.EntityID,
) (inventory.EntityID, bool) {
	if workload.Kind == kube.KindDeployment {
		return Snapshot{Model: model}.newestReplicaSet(workload)
	}
	return controllerNewest(model, workload)
}

// servingNow reports that the current revision serves, judged from
// the observed entity: a pod must be ready and of that revision; a
// Deployment's ReplicaSet must have ready replicas; any other entity of
// a StatefulSet or DaemonSet needs a ready pod of the revision.
func servingNow(
	model inventory.Reader, id, workload, current inventory.EntityID,
) bool {
	switch {
	case id.Kind == kube.KindPod && workload.Kind == kube.KindDeployment:
		return podReady(model, id) && slices.Contains(model.Related(id,
			inventory.OwnedBy, inventory.Outgoing), current)
	case id.Kind == kube.KindPod:
		return podReady(model, id) &&
			podOfRevision(model, id, workload, current)
	case workload.Kind == kube.KindDeployment:
		a, ok := model.Attribute(current, kube.AttrReadyReplicas)
		ready, _ := a.Value.AsNumber()
		return ok && ready > 0
	}
	return revisionServed(model, workload, current)
}

// servingWorkloadOf finds the Deployment, StatefulSet or DaemonSet an
// entity is, holds a revision of, or runs a pod of.
func servingWorkloadOf(
	model inventory.Reader, id inventory.EntityID,
) (inventory.EntityID, bool) {
	switch id.Kind {
	case kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet:
		return id, true
	case kube.KindReplicaSet, kube.KindControllerRevision:
		for _, owner := range model.Related(id, inventory.OwnedBy,
			inventory.Outgoing) {
			if owner.Kind == kube.KindDeployment || revisionKind(owner.Kind) {
				return owner, true
			}
		}
	case kube.KindPod:
		if deployment, ok := deploymentOf(model, id); ok {
			return deployment, true
		}
		return revisionWorkloadOf(model, id)
	}
	return inventory.EntityID{}, false
}

// rememberedServing reports that kwatch saw the workload's revision
// name serving within inventory.ServedRetention of the snapshot.
func (s Snapshot) rememberedServing(
	workload inventory.EntityID, name string,
) bool {
	memory, ok := s.Model.(servedReader)
	if !ok || name == "" {
		return false
	}
	var since time.Time
	if !s.Now.IsZero() {
		since = s.Now.Add(-inventory.ServedRetention)
	}
	for _, served := range memory.ServedRevisions(workload, since) {
		if served.Name == name {
			return true
		}
	}
	return false
}
