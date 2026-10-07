package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxVolumeUsers bounds the workloads named as users of a claim.
const maxVolumeUsers = 3

// volumeDetail says what a message about a nearly full claim names: its
// size and the workloads that mount it.
func volumeDetail(
	ctx detection.Context, e inventory.Entity,
) []detection.Evidence {
	var out []detection.Evidence
	used, hasUsed := number(e, kube.AttrVolumeUsedBytes)
	capacity, hasCap := number(e, kube.AttrVolumeCapacityBytes)
	if hasUsed && hasCap && capacity > 0 {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceVolumeSize,
			Value: quantity(used) + " of " + quantity(capacity),
		})
	}
	if users := volumeUsers(ctx, e.ID); users != "" {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceVolumeUsedBy, Value: users,
		})
	}
	return out
}

// volumeUsers lists the workloads whose pods mount the claim.
func volumeUsers(ctx detection.Context, claim inventory.EntityID) string {
	var names []string
	for _, pod := range ctx.Model.Related(
		claim, inventory.Mounts, inventory.Incoming) {
		names = append(names, inventory.TopOwner(ctx.Model, pod).Name)
	}
	return format.JoinNames(names, maxVolumeUsers)
}
