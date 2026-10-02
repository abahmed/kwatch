package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// budgetBlocksDrain reports a PodDisruptionBudget that allows no
// eviction while a pod it protects sits on a cordoned node for longer
// than DefaultBudgetBlocked. The pods are healthy; the budget is simply
// as strict as the replica count (minAvailable 1 of 1), so the drain
// waits forever.
func budgetBlocksDrain(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	allowed, known := number(e, kube.AttrDisruptionsAllowed)
	if !known || allowed > 0 || ctx.Model == nil {
		return detection.Finding{}, false
	}
	for _, pod := range budgetPods(ctx, e.ID) {
		node, since, ok := cordonedNodeOf(ctx, pod)
		if !ok {
			continue
		}
		key := "pdb-blocks-drain/" + pod.Name
		if !sustained(ctx, key, since, drainWait(e)) {
			continue
		}
		return detection.Finding{
			Reason: reasons.PdbBlocksDrain, Severity: detection.Warning,
			Health: detection.Failing, Since: since,
			// No semicolon: messages keep only a summary's first
			// clause, and the blocked drain is the news.
			Summary: "Disruption budget allows no eviction, so the " +
				"drain of node " + node.Name + " is blocked",
			Evidence: []detection.Evidence{{Label: "pod", Value: pod.Name}},
		}, true
	}
	return detection.Finding{}, false
}

func podLeaving(pod inventory.Entity) bool {
	return podFinished(pod) || flag(pod, kube.AttrDeleting)
}

func podFinished(pod inventory.Entity) bool {
	phase := text(pod, kube.AttrPhase)
	return phase == "Succeeded" || phase == "Failed"
}

// drainWait is how long a budget may block a drain before it is
// reported. A budget whose expected pods are all healthy while it allows
// no eviction is too strict for its replica count, which is conclusive;
// otherwise a replacement may still be starting, and the drain gets the
// full DefaultBudgetBlocked.
func drainWait(e inventory.Entity) time.Duration {
	healthy, ok := number(e, kube.AttrCurrentHealthy)
	expected, known := number(e, kube.AttrExpectedPods)
	if ok && known && expected > 0 && healthy >= expected {
		return DefaultBudgetTooStrict
	}
	return DefaultBudgetBlocked
}

// cordonedNodeOf returns the node a pod runs on when that node is
// cordoned, with the time it was cordoned. A finished or terminating
// pod does not hold up a drain.
func cordonedNodeOf(
	ctx detection.Context, pod inventory.EntityID,
) (inventory.EntityID, time.Time, bool) {
	if p, ok := ctx.Model.Entity(pod); ok && podLeaving(p) {
		return inventory.EntityID{}, time.Time{}, false
	}
	for _, id := range ctx.Model.Related(pod, inventory.RunsOn,
		inventory.Outgoing) {
		node, ok := ctx.Model.Entity(id)
		if ok && flag(node, kube.AttrUnschedulable) {
			return id, valueSince(node, kube.AttrUnschedulable), true
		}
	}
	return inventory.EntityID{}, time.Time{}, false
}
