package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// Preemption storm thresholds.
const (
	preemptionByScheduler = "PreemptionByScheduler"
	preemptionStorm       = 3
	preemptionWindow      = 15 * time.Minute
)

// preemptionFindings reports a pod preempted by the scheduler when at
// least preemptionStorm pods of the same owner were preempted within
// preemptionWindow: its workload keeps losing capacity to higher-priority
// pods. Only pods still in the model count; a victim leaves it when its
// deletion completes, so the storm is seen while victims terminate.
func preemptionFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if !preemptedAt(ctx, e) || ctx.Model == nil {
		return nil
	}
	owners := ctx.Model.Related(e.ID, inventory.OwnedBy, inventory.Outgoing)
	if len(owners) == 0 {
		return nil
	}
	victims := 0
	for _, id := range ctx.Model.Related(
		owners[0], inventory.OwnedBy, inventory.Incoming,
	) {
		if pod, ok := ctx.Model.Entity(id); ok && preemptedAt(ctx, pod) {
			victims++
		}
	}
	if victims < preemptionStorm {
		return nil
	}
	_, _, since := condition(e, "DisruptionTarget")
	return []detection.Finding{{
		Reason: reasons.PodPreemptedRepeatedly, Severity: detection.Warning,
		Since: since,
		Summary: "Pods of " + owners[0].Name + " keep being preempted by " +
			"higher-priority pods",
		Evidence: []detection.Evidence{{
			Label: "preempted pods", Value: strconv.Itoa(victims),
		}},
	}}
}

// preemptedAt reports a pod the scheduler preempted within the window.
func preemptedAt(ctx detection.Context, e inventory.Entity) bool {
	status, reason, since := condition(e, "DisruptionTarget")
	return status == "True" && reason == preemptionByScheduler &&
		(since.IsZero() || ctx.Now.Sub(since) <= preemptionWindow)
}
