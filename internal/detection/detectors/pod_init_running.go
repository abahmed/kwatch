package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// initRunning reports a pod with an init container running now, or a
// sidecar still within its startup probe (sidecarStarting).
func initRunning(ctx detection.Context, pod inventory.EntityID) bool {
	if ctx.Model == nil {
		return false
	}
	for _, id := range ctx.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		c, ok := ctx.Model.Entity(id)
		if !ok || text(c, kube.AttrState) != "running" {
			continue
		}
		if flag(c, kube.AttrInit) || sidecarStarting(ctx, c) {
			return true
		}
	}
	return false
}

// sidecarStarting reports a sidecar that runs and has not passed its
// startup probe yet, within the time that probe allows: the pod is
// starting. Past that budget the kubelet restarts the sidecar, which
// the container findings report, so the pod's own wait counts again.
func sidecarStarting(ctx detection.Context, c inventory.Entity) bool {
	started := timestamp(c, kube.AttrStartedAt)
	if !flag(c, kube.AttrSidecarStarting) || started.IsZero() {
		return false
	}
	budget := DefaultInitWait
	if seconds, ok := number(c, kube.AttrProbeBudget); ok && seconds > 0 {
		budget = time.Duration(seconds) * time.Second
	}
	left := started.Add(budget).Sub(ctx.Now)
	if left <= 0 {
		return false
	}
	ctx.RecheckAfter(left)
	return true
}
