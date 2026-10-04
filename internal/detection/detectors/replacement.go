package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Replacement grace. A pod that lands on a node younger than freshNode
// is a replacement after a scale-up or a consolidation: the node pulls
// its images cold and starts every pod at once. Such a pod, and the
// workload waiting for it, get replacementGrace on top of their usual
// thresholds before anything is reported.
const (
	freshNode        = 10 * time.Minute
	youngPod         = 10 * time.Minute
	replacementGrace = 5 * time.Minute
)

// replacementGraceFor is the extra time a pod gets while it starts on
// a fresh node, or zero.
func replacementGraceFor(
	ctx detection.Context, pod inventory.Entity,
) time.Duration {
	created := timestamp(pod, kube.AttrCreated)
	if created.IsZero() || ctx.Now.Sub(created) > youngPod {
		return 0
	}
	for _, id := range ctx.Model.Related(pod.ID, inventory.RunsOn,
		inventory.Outgoing) {
		node, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if born := timestamp(node, kube.AttrCreated); !born.IsZero() &&
			ctx.Now.Sub(born) <= freshNode {
			return replacementGrace
		}
	}
	return 0
}

// workloadReplacementGrace is the extra time a workload gets while any
// of its running pods starts on a fresh node, or zero.
func workloadReplacementGrace(
	ctx detection.Context, workload inventory.EntityID,
) time.Duration {
	if ctx.Model == nil {
		return 0
	}
	for _, id := range runningPodsOf(ctx.Model, workload) {
		pod, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if grace := replacementGraceFor(ctx, pod); grace > 0 {
			return grace
		}
	}
	return 0
}
