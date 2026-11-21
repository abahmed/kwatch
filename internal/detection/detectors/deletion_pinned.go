package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// heldByBoundClaim reports a PersistentVolume whose deletion waits for
// a claim that is still bound and not being deleted.
func heldByBoundClaim(ctx detection.Context, e inventory.Entity) bool {
	if e.ID.Kind != kube.KindPV || ctx.Model == nil {
		return false
	}
	for _, id := range ctx.Model.Related(
		e.ID, inventory.References, inventory.Incoming) {
		claim, ok := ctx.Model.Entity(id)
		if id.Kind == kube.KindPVC && ok &&
			text(claim, kube.AttrPhase) == "Bound" &&
			!flag(claim, kube.AttrDeleting) {
			return true
		}
	}
	return false
}
