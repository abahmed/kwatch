package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// fleetSilentGrace is how long every node going silent at once is held
// before it is reported. All kubelets stopping together points at the
// path they report through, which usually returns within minutes.
const fleetSilentGrace = 4 * time.Minute

// fleetSilent reports whether two or more nodes exist and every one of
// them reads Ready=Unknown.
func fleetSilent(ctx detection.Context) bool {
	if ctx.Model == nil {
		return false
	}
	ids := ctx.Model.EntitiesIn(kube.KindNode, "")
	if len(ids) < 2 {
		return false
	}
	for _, id := range ids {
		n, ok := ctx.Model.Entity(id)
		if !ok {
			return false
		}
		if status, _, _ := condition(n, "Ready"); status != "Unknown" {
			return false
		}
	}
	return true
}

// silentGrace is the wait before a silent node is reported: the longer
// fleetSilentGrace while the whole fleet is silent.
func silentGrace(ctx detection.Context, grace time.Duration) time.Duration {
	if grace < fleetSilentGrace && fleetSilent(ctx) {
		return fleetSilentGrace
	}
	return grace
}
