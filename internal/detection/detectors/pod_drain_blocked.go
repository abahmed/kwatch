package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// unownedBlocksDrain reports a pod no controller owns that stays on a
// cordoned node for longer than DefaultBudgetBlocked. A drain evicts it
// and nothing recreates it, so the drain waits for someone to remove it.
func unownedBlocksDrain(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if ctx.Model == nil || text(e, kube.AttrPhase) != "Running" ||
		len(ctx.Model.Related(e.ID, inventory.OwnedBy,
			inventory.Outgoing)) > 0 {
		return detection.Finding{}, false
	}
	node, since, ok := cordonedNodeOf(ctx, e.ID)
	if !ok || !sustained(ctx, "pod-blocks-drain", since,
		DefaultBudgetBlocked) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.PodBlocksDrain, Severity: detection.Warning,
		Health: detection.Degraded, Since: since,
		Summary: "Pod has no controller, so the drain of node " +
			node.Name + " cannot move it",
	}, true
}
