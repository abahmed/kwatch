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
// so the kubelet's retries fail for a minute or two and then work. That
// includes address assignment: an address manager that is still warming
// its pool answers "failed to assign an IP address" until it has one, so
// the words of an exhausted pool on a young node are warm-up, not
// exhaustion. The event is quoted again once the node is old, if the pod
// still has not started: a pool that is really empty stays empty.
func youngNodeNoise(
	ctx detection.Context, pod inventory.Entity, note inventory.Note,
) bool {
	if ctx.Model == nil || !bootReasons[note.Reason] {
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

// youngNodeHold reports a node still inside its boot window and asks for
// a recheck when the window ends. Its pods' network failures are the
// warm-up of its network plugin, so they make no node finding yet.
func youngNodeHold(ctx detection.Context, node inventory.EntityID) bool {
	remaining := kube.NodeYoungRemaining(ctx.Model, node, ctx.Now)
	if remaining <= 0 {
		return false
	}
	ctx.RecheckAfter(remaining)
	return true
}
