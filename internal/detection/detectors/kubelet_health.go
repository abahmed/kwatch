package detectors

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Kubelet health thresholds, read from the kubelet's own metrics.
const (
	// plegSlowMS is the mean pod lifecycle relist time above which the
	// kubelet is falling behind its pods; the kubelet itself turns
	// NotReady only after three minutes without any relist.
	plegSlowMS = 1000.0
	// plegSustained is how long the relist must stay slow.
	plegSustained = 5 * time.Minute
)

// kubeletFindings read the kubelet's pod lifecycle relist time and its
// evictions. A slow relist is a kubelet that cannot keep up; evictions
// are the kubelet reclaiming resources from pods right now.
func kubeletFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	if relist, ok := number(e, kube.AttrPLEGRelistMS); ok &&
		relist >= plegSlowMS {
		since := ctx.Onset("pleg-slow", valueSince(e, kube.AttrPLEGRelistMS))
		if sustained(ctx, "pleg-slow", since, plegSustained) {
			out = append(out, detection.Finding{
				Reason: reasons.NodePLEGSlow, Severity: detection.Warning,
				Since: since,
				Summary: "The kubelet takes " + format.Duration(
					time.Duration(relist)*time.Millisecond) +
					" to list its pods; it is falling behind them",
			})
		}
	}
	if rate, ok := number(e, kube.AttrEvictionRate); ok && rate > 0 {
		out = append(out, detection.Finding{
			Reason: reasons.NodeEvicting, Severity: detection.Info,
			Since:   ctx.Onset("evicting", valueSince(e, kube.AttrEvictionRate)),
			Summary: "The kubelet is evicting pods to reclaim resources",
			Evidence: []detection.Evidence{{Label: "evictions per minute",
				Value: fmt.Sprintf("%.1f", rate*60)}},
		})
	}
	return out
}
