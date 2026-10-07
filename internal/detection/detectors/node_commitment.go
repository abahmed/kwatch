package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// memoryOvercommit is the share of a node's allocatable memory that the
// memory limits of its pods may add up to before the node counts as
// overcommitted. Limits above requests are normal; limits at 150% of
// the node mean that under load the kernel kills pods that stayed
// within their own limit.
const memoryOvercommit = 1.5

// memoryOvercommitClear is the share a raised finding must fall below to
// end, and overcommitSustain is how long the share must stay at
// memoryOvercommit before the finding is raised. Pods come and go, so a
// share that touches 150% for one evaluation says little.
const (
	memoryOvercommitClear = 1.4
	overcommitSustain     = 5 * time.Minute
)

// NodeCommitment detects nodes that are short of memory (pressure,
// evictions or a kernel OOM) while their pods' memory limits add up to
// far more than the node has. It explains OOM kills and evictions of pods
// that did nothing wrong on their own. Limits alone are never reported.
type NodeCommitment struct{}

// Name implements detection.Detector.
func (NodeCommitment) Name() string { return "node-commitment" }

// Kinds implements detection.Detector.
func (NodeCommitment) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindNode}
}

// Detect implements detection.Detector.
func (NodeCommitment) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	allocatable, ok := number(e, kube.AttrMemoryAllocatable)
	if !ok || allocatable <= 0 || nodeIsYoung(ctx, e) {
		return nil
	}
	limits, unlimited := memoryLimitsOn(ctx, e.ID)
	share := limits / allocatable
	level := memoryOvercommit
	if ctx.Ongoing("memory-overcommit") {
		level = memoryOvercommitClear
	}
	if share < level {
		return nil
	}
	if !sustained(ctx, "memory-overcommit", time.Time{},
		overcommitSustain) {
		return nil
	}
	// Limits above the node's memory are normal, and a risk is not news.
	// The commitment is reported only while the node is short of memory,
	// as the reason pods on it are killed.
	if !nodeShortOnMemory(e) && !systemOOMNear(ctx, e, ctx.Now) {
		return nil
	}
	since := ctx.Onset("memory-overcommit", ctx.Now)
	evidence := []detection.Evidence{
		{Label: "memory limits", Value: percentText(share*100) +
			" of allocatable"},
	}
	if unlimited > 0 {
		evidence = append(evidence, detection.Evidence{
			Label: "containers without a memory limit",
			Value: strconv.Itoa(unlimited)})
	}
	return []detection.Finding{{
		Reason: reasons.NodeMemoryOvercommitted, Severity: detection.Warning,
		Since: since,
		Summary: "Node is short on memory and its pods' limits add up " +
			"to " + percentText(share*100) + " of its memory, so pods " +
			"that stayed within their own limit are killed",
		Evidence: evidence,
	}}
}

// memoryLimitsOn sums the memory limits of the app containers of the
// pods running on node, and counts the containers that have none. Pods
// that finished and regular init containers hold no memory (the same
// rule NodeHealth applies, through runningContainers).
func memoryLimitsOn(
	ctx detection.Context, node inventory.EntityID,
) (limits float64, unlimited int) {
	for _, pod := range ctx.Model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		if entity, ok := ctx.Model.Entity(pod); ok && podFinished(entity) {
			continue
		}
		for _, container := range runningContainers(ctx, pod) {
			limit, ok := number(container, kube.AttrMemoryLimit)
			if !ok {
				unlimited++
				continue
			}
			limits += limit
		}
	}
	return limits, unlimited
}
