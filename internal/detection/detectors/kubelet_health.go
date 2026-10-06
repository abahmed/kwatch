package detectors

import (
	"fmt"
	"strconv"
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
	// kubeletFailuresMin is how many failed reads within
	// kube.KubeletFailureWindow make a node worth reporting: one or two
	// are a network blip, three are a pattern.
	kubeletFailuresMin = 3
	// kubeletFailureSpanMin is how long those failures must stretch: a
	// few failures within minutes are one blip, not a pattern.
	kubeletFailureSpanMin = 30 * time.Minute
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
	if f, ok := kubeletUnreachable(ctx, e); ok {
		out = append(out, f)
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

// kubeletUnreachable reports a node the API server calls Ready but whose
// kubelet kwatch keeps failing to read (and has not read since), so its
// usage and pressure metrics are missing. A node that is not Ready is
// already reported as such. It is informational: the node works, kwatch
// cannot see all of it.
func kubeletUnreachable(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	failures, ok := number(e, kube.AttrKubeletFailures)
	spanSeconds, _ := number(e, kube.AttrKubeletFailureSpan)
	span := time.Duration(spanSeconds * float64(time.Second))
	if status, _, _ := condition(e, "Ready"); !ok || status != "True" ||
		failures < kubeletFailuresMin || span < kubeletFailureSpanMin {
		return detection.Finding{}, false
	}
	count := strconv.Itoa(int(failures))
	return detection.Finding{
		Reason: reasons.KubeletUnreachable, Severity: detection.Info,
		// The count changes with every failure; the onset must not.
		Since: ctx.Onset("kubelet-unreachable",
			valueSince(e, kube.AttrKubeletFailures)),
		Summary: "kwatch could not reach the kubelet on node " +
			e.ID.Name + " " + count + " times in the last 6 hours; node " +
			"metrics for it are missing.",
		Evidence: []detection.Evidence{{
			Label: "failed reads in the last 6 hours", Value: count}},
	}, true
}
