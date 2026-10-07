package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// bootGraceFor is the extra time a pod that is only starting gets
// while its node pool boots (see kube.BootWindow), or zero. The grace
// is as long as a whole boot and ends the moment the pool stops
// booting, so a pod still failing then is reported at once, with its
// usual thresholds already spent. A pod no node holds yet waits for
// capacity: it is expected while any pool boots. A pod whose container
// crashes or restarts, cannot pull its image or has a bad
// configuration is not starting, it is failing, and gets nothing.
func bootGraceFor(
	ctx detection.Context, pod inventory.Entity,
) time.Duration {
	if ctx.Model == nil || podFailingToRun(ctx, pod.ID) {
		return 0
	}
	remaining := podNodeBootRemaining(ctx, pod)
	if remaining <= 0 {
		return 0
	}
	// Look again when the boot is over: nothing else wakes the pod up.
	ctx.RecheckAfter(remaining)
	return max(kube.BootWindow, wakeHold(ctx, pod))
}

// wakeHold is how long a pod the wake-up created may stay behind its
// usual thresholds: its whole age so far and what is left of the
// wake-up, so what still fails when the wake-up ends is reported at
// once. It is zero for any other pod.
func wakeHold(ctx detection.Context, pod inventory.Entity) time.Duration {
	remaining := kube.PodWakeRemaining(ctx.Model, pod, ctx.Now)
	if remaining <= 0 {
		return 0
	}
	created, _ := pod.Attribute(kube.AttrCreated)
	return remaining + max(0, ctx.Now.Sub(created.Value.AsTime()))
}

// podNodeBootRemaining is how much longer the pod is expected to start
// slowly: while the pool of its node boots, and while the cluster wakes
// up if the pod was created by the wake-up. A pod no node holds yet
// waits for capacity: it is expected while any pool boots.
func podNodeBootRemaining(
	ctx detection.Context, pod inventory.Entity,
) time.Duration {
	wake := kube.PodWakeRemaining(ctx.Model, pod, ctx.Now)
	nodes := ctx.Model.Related(pod.ID, inventory.RunsOn, inventory.Outgoing)
	if len(nodes) == 0 {
		return max(wake, kube.ClusterBootRemaining(ctx.Model, ctx.Now))
	}
	remaining := wake
	for _, node := range nodes {
		remaining = max(remaining,
			kube.NodeBootRemaining(ctx.Model, node, ctx.Now))
	}
	return remaining
}

// podFailingToRun reports a container of the pod in a failure that a
// slow boot does not explain.
func podFailingToRun(ctx detection.Context, pod inventory.EntityID) bool {
	for _, id := range ctx.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		container, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		// A container that restarted has already crashed or been
		// killed: it is not slow, it is failing.
		restarts, _ := number(container, kube.AttrRestarts)
		if _, failing := waitingReasons[text(container,
			kube.AttrStateReason)]; failing || restarts > 0 {
			return true
		}
	}
	return false
}

// workloadBootGrace is bootGraceFor for a workload: it applies only
// when every pod of it that is not ready is waiting on a booting pool.
// One pod failing for its own reasons keeps the usual threshold.
func workloadBootGrace(
	ctx detection.Context, workload inventory.EntityID,
) time.Duration {
	var longest time.Duration
	for _, id := range runningPodsOf(ctx.Model, workload) {
		pod, ok := ctx.Model.Entity(id)
		if !ok || flag(pod, kube.AttrReady) {
			continue
		}
		grace := bootGraceFor(ctx, pod)
		if grace == 0 {
			return 0
		}
		longest = max(longest, grace)
	}
	return longest
}

// nodeIsYoung holds back an advisory about a node that is younger than
// kube.BootWindow: a fresh node is busy pulling images and starting
// every pod at once, so commitment and pressure-stall readings are
// expected. It asks to look again when the node turns old; what still
// holds then is reported at once.
func nodeIsYoung(ctx detection.Context, node inventory.Entity) bool {
	if ctx.Model == nil {
		return false
	}
	remaining := kube.NodeYoungRemaining(ctx.Model, node.ID, ctx.Now)
	if remaining <= 0 {
		return false
	}
	ctx.RecheckAfter(remaining)
	return true
}
