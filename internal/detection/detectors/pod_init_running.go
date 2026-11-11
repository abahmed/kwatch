package detectors

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// initRunning reports a pod with an init container running now.
func initRunning(ctx detection.Context, pod inventory.EntityID) bool {
	if ctx.Model == nil {
		return false
	}
	for _, id := range ctx.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		c, ok := ctx.Model.Entity(id)
		if ok && flag(c, kube.AttrInit) &&
			text(c, kube.AttrState) == "running" {
			return true
		}
	}
	return false
}
