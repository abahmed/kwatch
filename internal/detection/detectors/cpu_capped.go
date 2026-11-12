package detectors

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// cpuSpareShare is the share of a node's allocatable CPU in use below
// which the node is not short of CPU: a stall it reports then comes
// from containers the kernel holds at their own CPU limits.
const cpuSpareShare = 50.0

// stallFromLimits reports a node CPU stall that the node's own load
// cannot explain: the node uses under cpuSpareShare of its allocatable
// CPU while a container on it runs at its CPU limit. The kernel
// throttles that container, and its waiting tasks count in the node's
// CPU pressure, so the node looks stalled with CPU to spare. The
// container is reported on its own (cpuCappedFinding, or the throttled
// finding); the node is not the problem.
func stallFromLimits(ctx detection.Context, node inventory.Entity) bool {
	allocatable, ok := number(node, kube.AttrCPUAllocatable)
	if !ok || allocatable <= 0 {
		return false
	}
	used, seen, capped := 0.0, false, false
	for _, pod := range ctx.Model.Related(node.ID, inventory.RunsOn,
		inventory.Incoming) {
		for _, c := range runningContainers(ctx, pod) {
			if milli, ok := number(c, kube.AttrCPUUsageMilli); ok {
				used, seen = used+milli, true
			}
			capped = capped || atCPULimit(c)
		}
	}
	return seen && capped && 100*used/allocatable < cpuSpareShare
}

// atCPULimit reports a container the kernel holds at its CPU limit:
// heavily throttled, or using nearly all of its limit.
func atCPULimit(c inventory.Entity) bool {
	if pct, ok := number(c, kube.AttrThrottledPct); ok &&
		pct >= throttleWarning {
		return true
	}
	pct, ok := ratio(c, kube.AttrCPUUsageMilli, kube.AttrCPULimit)
	return ok && pct >= limitWarning
}

// cpuCappedFinding reports a container held at its CPU limit while its
// node's tasks stall on CPU: the stall is this container waiting for
// CPU time its limit does not give, not a node short of CPU. A
// container with a heavy throttled share is reported by the throttled
// finding instead. Like that finding, it is only a warning: requests
// slow down, nothing fails.
func cpuCappedFinding(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if pct, ok := number(e, kube.AttrThrottledPct); ok &&
		pct >= throttleWarning {
		return detection.Finding{}, false
	}
	pct, ok := ratio(e, kube.AttrCPUUsageMilli, kube.AttrCPULimit)
	if !ok || pct < limitWarning || !nodeStallsOnCPU(ctx, e.ID) {
		return detection.Finding{}, false
	}
	since := ctx.Onset("cpu-capped",
		valueSince(e, kube.AttrCPUUsageMilli))
	if !sustained(ctx, "cpu-capped", since, psiSustain) {
		return detection.Finding{}, false
	}
	used, _ := number(e, kube.AttrCPUUsageMilli)
	limit, _ := number(e, kube.AttrCPULimit)
	return detection.Finding{
		Reason: reasons.ContainerCPUThrottled, Severity: detection.Warning,
		Since: since,
		Summary: "CPU is held at its limit (" + milliText(used) + " of " +
			milliText(limit) + "); its tasks wait for CPU and requests " +
			"slow down",
		Evidence: []detection.Evidence{
			{Label: detection.EvidenceCPULimit, Value: milliText(limit)},
			{Label: "cpu used", Value: milliText(used)},
		},
	}, true
}

// nodeStallsOnCPU reports whether the node a container runs on says its
// tasks stall on CPU.
func nodeStallsOnCPU(
	ctx detection.Context, container inventory.EntityID,
) bool {
	for _, pod := range ctx.Model.Related(container, inventory.PartOf,
		inventory.Outgoing) {
		for _, id := range ctx.Model.Related(pod, inventory.RunsOn,
			inventory.Outgoing) {
			node, ok := ctx.Model.Entity(id)
			if stall, found := number(node, kube.AttrCPUPSI); ok && found &&
				stall >= psiThreshold {
				return true
			}
		}
	}
	return false
}

// milliText writes CPU in millicores, as limits are written: "250m".
func milliText(milli float64) string {
	return strconv.Itoa(int(milli+0.5)) + "m"
}
