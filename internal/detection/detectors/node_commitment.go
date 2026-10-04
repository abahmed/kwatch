package detectors

import (
	"strconv"

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

// NodeCommitment detects nodes whose pods' memory limits add up to far
// more than the node has. It explains OOM kills and evictions of pods
// that did nothing wrong on their own.
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
	if !ok || allocatable <= 0 {
		return nil
	}
	limits, unlimited := memoryLimitsOn(ctx.Model, e.ID)
	share := limits / allocatable
	if share < memoryOvercommit {
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
		Summary: "Memory limits of the pods on this node add up to " +
			percentText(share*100) + " of its memory; under load the " +
			"kernel will kill pods that stayed within their own limit",
		Evidence: evidence,
	}}
}

// memoryLimitsOn sums the memory limits of the containers of the pods
// running on node, and counts the containers that have none. Pods that
// finished no longer hold memory.
func memoryLimitsOn(
	model inventory.Reader, node inventory.EntityID,
) (limits float64, unlimited int) {
	for _, pod := range model.Related(node, inventory.RunsOn,
		inventory.Incoming) {
		if entity, ok := model.Entity(pod); ok && podFinished(entity) {
			continue
		}
		for _, id := range model.Related(pod, inventory.PartOf,
			inventory.Incoming) {
			container, ok := model.Entity(id)
			if !ok {
				continue
			}
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
