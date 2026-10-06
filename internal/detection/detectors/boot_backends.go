package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// backendsAreStarting reports a Service whose pods that are not ready
// are all still starting, so that having no ready endpoints is
// expected. A pod is starting while its node pool boots, or while it
// is younger than kube.BootWindow; a pod that crashes or restarts is
// failing and ends the grace. It asks to look again when the first of
// the pods stops being excused, so a Service still empty then is
// reported at once.
func backendsAreStarting(
	ctx detection.Context, service inventory.EntityID,
) bool {
	if ctx.Model == nil {
		return false
	}
	var first time.Duration
	held := false
	for _, id := range budgetPods(ctx, service) {
		pod, ok := ctx.Model.Entity(id)
		if !ok || podFinished(pod) || flag(pod, kube.AttrReady) {
			continue
		}
		remaining := podStartingRemaining(ctx, pod)
		if remaining <= 0 {
			return false
		}
		if !held || remaining < first {
			first = remaining
		}
		held = true
	}
	if held {
		ctx.RecheckAfter(first)
	}
	return held
}

// podStartingRemaining is how much longer a not-ready pod is excused
// as still starting, or zero when it is failing or has had its time.
func podStartingRemaining(
	ctx detection.Context, pod inventory.Entity,
) time.Duration {
	if podFailingToRun(ctx, pod.ID) {
		return 0
	}
	remaining := podNodeBootRemaining(ctx, pod)
	if created, ok := pod.Attribute(kube.AttrCreated); ok &&
		!created.Value.AsTime().IsZero() {
		age := ctx.Now.Sub(created.Value.AsTime())
		remaining = max(remaining, kube.BootWindow-age)
	}
	return remaining
}
