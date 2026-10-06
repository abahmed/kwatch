package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A rollout that is moving normally changes things on purpose: old pods
// terminate, new pods start and are not ready yet, and for a while the
// workload has fewer ready replicas than desired. None of that is a
// failure. These helpers say when a rollout is that kind of normal, so
// the findings that only restate the transition can wait.
//
// The hold never covers what a broken rollout shows. A new pod that
// crashes or cannot start has container findings of its own, which are
// not held. A rollout the controller gave up on says so
// (ProgressDeadlineExceeded) and a StatefulSet rollout that stops for
// DefaultRolloutStuck is its own finding. A pod that is still not ready
// after its start budget is reported by the pod detector as before.
const (
	// rolloutHoldMax caps how long a Deployment rollout is held from
	// the creation of its newest ReplicaSet. The controller reports a
	// missed deadline long before; the cap only guards a controller
	// that stopped updating the status.
	rolloutHoldMax = 30 * time.Minute
	// rolloutStartBudget is how long a pod of a rolling workload may
	// take to pass its readiness probe before probe failures count.
	rolloutStartBudget = DefaultNotReady
)

// progressReasons are the Progressing reasons of a Deployment whose
// rollout is under way and inside its deadline.
var progressReasons = map[string]bool{
	"ReplicaSetUpdated":    true,
	"NewReplicaSetCreated": true,
	"FoundNewReplicaSet":   true,
}

// rolloutProgressing reports a Deployment or StatefulSet whose rollout
// is moving and within its deadline. It asks to be looked at again when
// the hold can end.
func rolloutProgressing(
	ctx detection.Context, e inventory.Entity,
) bool {
	switch e.ID.Kind {
	case kube.KindDeployment:
		return deploymentProgressing(ctx, e)
	case kube.KindStatefulSet:
		return statefulSetProgressing(ctx, e)
	}
	return false
}

func deploymentProgressing(
	ctx detection.Context, e inventory.Entity,
) bool {
	status, reason, _ := condition(e, "Progressing")
	if status != "True" || !progressReasons[reason] || ctx.Model == nil {
		return false
	}
	started := newestReplicaSetCreated(ctx.Model, e.ID)
	if started.IsZero() {
		return false
	}
	remaining := rolloutHoldMax - ctx.Now.Sub(started)
	if remaining <= 0 {
		return false
	}
	ctx.RecheckAfter(remaining)
	return true
}

// newestReplicaSetCreated is when the Deployment's latest ReplicaSet
// was created: the start of the rollout, durable across restarts of
// kwatch.
func newestReplicaSetCreated(
	model inventory.Reader, deployment inventory.EntityID,
) time.Time {
	var newest time.Time
	for _, id := range model.Related(deployment, inventory.OwnedBy,
		inventory.Incoming) {
		if rs, ok := model.Entity(id); ok && id.Kind == kube.KindReplicaSet {
			newest = latest(newest, timestamp(rs, kube.AttrCreated))
		}
	}
	return newest
}

// statefulSetProgressing is a rolling update that moved within
// DefaultRolloutStuck: every updated pod renews the hold. A partition
// holds the rollout on purpose and is handled by the rollout finding.
func statefulSetProgressing(
	ctx detection.Context, e inventory.Entity,
) bool {
	update := text(e, kube.AttrRevision)
	current := text(e, kube.AttrCurrentRevision)
	if update == "" || current == "" || update == current ||
		text(e, kube.AttrUpdateStrategy) == "OnDelete" {
		return false
	}
	if partition, _ := number(e, kube.AttrPartition); partition > 0 {
		return false
	}
	moved := latest(valueSince(e, kube.AttrRevision),
		valueSince(e, kube.AttrUpdatedReplicas))
	remaining := DefaultRolloutStuck - ctx.Now.Sub(moved)
	if remaining <= 0 {
		return false
	}
	ctx.RecheckAfter(remaining)
	return true
}

// podStartingInRollout reports a young pod of a rolling workload that
// is still inside its start budget: its readiness probe failing now is
// part of starting. A pod that outlives the budget is a failure.
func podStartingInRollout(
	ctx detection.Context, pod inventory.Entity,
) bool {
	created := timestamp(pod, kube.AttrCreated)
	if ctx.Model == nil || created.IsZero() {
		return false
	}
	budget := max(rolloutStartBudget, startupBudget(ctx, pod.ID))
	age := ctx.Now.Sub(created)
	if age >= budget {
		return false
	}
	owner, ok := ctx.Model.Entity(inventory.TopOwner(ctx.Model, pod.ID))
	if !ok || !rolloutProgressing(ctx, owner) {
		return false
	}
	ctx.RecheckAfter(budget - age)
	return true
}
