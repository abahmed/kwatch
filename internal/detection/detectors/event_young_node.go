package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// youngNodeNoise reports a bootReasons event of a pod that runs on a node
// younger than kube.BootWindow. A node that joined to replace another has
// its network plugin and its agents start together with the first pods,
// so the kubelet's retries fail for a minute or two and then work. The
// event is quoted again once the node is old, if the pod still has not
// started. An exhausted address pool does not heal with the node's age,
// so it is never held.
func youngNodeNoise(
	ctx detection.Context, pod inventory.Entity, note inventory.Note,
) bool {
	if ctx.Model == nil || !bootReasons[note.Reason] ||
		eventMode(note) == detection.ModeNetworkIPExhausted {
		return false
	}
	var remaining time.Duration
	for _, node := range ctx.Model.Related(pod.ID, inventory.RunsOn,
		inventory.Outgoing) {
		remaining = max(remaining,
			kube.NodeYoungRemaining(ctx.Model, node, ctx.Now))
	}
	if remaining <= 0 {
		return false
	}
	ctx.RecheckAfter(remaining)
	return true
}
