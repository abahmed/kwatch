package detectors

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// throttleEvidence says how starved of CPU a container is when its
// probe fails: a probe handler that cannot get CPU time answers late.
// The stats poller already reads the throttled share of every container
// from cAdvisor, so this only reads it back. It says nothing for a
// container that is not throttled hard or has no reading.
func throttleEvidence(e inventory.Entity) []detection.Evidence {
	pct, ok := number(e, kube.AttrThrottledPct)
	if !ok || pct < throttleWarning {
		return nil
	}
	out := []detection.Evidence{{
		Label: detection.EvidenceCPUThrottled, Value: percentText(pct)}}
	if limit, ok := number(e, kube.AttrCPULimit); ok && limit > 0 {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceCPULimit,
			Value: strconv.Itoa(int(limit)) + "m"})
	}
	return out
}
