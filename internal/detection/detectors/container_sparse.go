package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// sparseRestartGap is the average time between restarts, over the pod's
// life, above which a container is not "repeatedly" restarting: a pod
// that restarted five times in three months and restarts once more is
// having a bad day, not a restart loop. A container that crashes every
// few hours for days stays well under it and is still reported.
const sparseRestartGap = 24 * time.Hour

// sparseRestarts reports whether the container's restart count is spread
// thin over a long life. The restart count is cumulative, so without
// this one more restart of a long-lived pod reads as a loop. When the
// pod's start is unknown it says nothing, and the count stands.
func sparseRestarts(
	ctx detection.Context, e inventory.Entity, restarts float64,
	lastRestart time.Time,
) bool {
	pod, ok := owningPod(ctx, e)
	if !ok || restarts <= 0 {
		return false
	}
	start := timestamp(pod, kube.AttrStartTime)
	if start.IsZero() {
		start = timestamp(pod, kube.AttrCreated)
	}
	life := lastRestart.Sub(start)
	if start.IsZero() || life <= 0 {
		return false
	}
	return life/time.Duration(restarts) > sparseRestartGap
}
