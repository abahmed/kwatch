package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// scaleUpSlack is how much older than the observed scale change a pod
// may be and still count as added by it: the change is dated when
// kwatch saw it, the pod by the cluster's clock.
const scaleUpSlack = time.Minute

// statefulSetGrowing reports a StatefulSet that is short of its desired
// replicas only because it is still adding them, and is moving. An
// ordered set adds one pod at a time and waits for each to be ready,
// so a large scale-up, or a first creation, is below its size for many
// minutes while nothing is wrong. It is growing when
//
//   - it fell short (short) when its desired count last changed, not
//     later: replicas were asked for, none was lost;
//   - it has added a pod since, and every pod that is not ready is one
//     of those;
//   - a pod was created or became ready within DefaultRolloutStuck.
//
// The hold never covers an outage: a pod from before the scale-up that
// is not ready ends it, and so does a new pod that stays unready until
// nothing has moved for DefaultRolloutStuck. A pod that crashes or
// cannot start has findings of its own, which are not held.
func statefulSetGrowing(
	ctx detection.Context, e inventory.Entity, short time.Time,
) bool {
	asked := valueSince(e, kube.AttrReplicas)
	if e.ID.Kind != kube.KindStatefulSet || ctx.Model == nil ||
		asked.IsZero() || short.After(asked.Add(scaleUpSlack)) {
		return false
	}
	added := false
	moved := latest(asked, valueSince(e, kube.AttrReadyReplicas))
	for _, id := range ctx.Model.Related(e.ID, inventory.OwnedBy,
		inventory.Incoming) {
		pod, ok := ctx.Model.Entity(id)
		if !ok || id.Kind != kube.KindPod {
			continue
		}
		created := timestamp(pod, kube.AttrCreated)
		isNew := !created.Before(asked.Add(-scaleUpSlack))
		if !flag(pod, kube.AttrReady) && !isNew {
			return false
		}
		added = added || isNew
		moved = latest(moved, created)
	}
	return added && holdFor(ctx, DefaultRolloutStuck-ctx.Now.Sub(moved))
}

// holdFor reports whether a hold has time left and, when it has, asks
// to be looked at again as it ends.
func holdFor(ctx detection.Context, remaining time.Duration) bool {
	if remaining <= 0 {
		return false
	}
	ctx.RecheckAfter(remaining)
	return true
}
