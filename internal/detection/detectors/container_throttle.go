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
// from cAdvisor, so this only reads it back. A container throttled hard
// gets its throttled share; one that uses nearly all of its limit
// without that share (the kernel holds it at the limit) gets what it
// uses. It says nothing for a container that is not at its limit or has
// no reading.
func throttleEvidence(e inventory.Entity) []detection.Evidence {
	limit, limited := number(e, kube.AttrCPULimit)
	limited = limited && limit > 0
	var out []detection.Evidence
	if pct, ok := number(e, kube.AttrThrottledPct); ok &&
		pct >= throttleWarning {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceCPUThrottled, Value: percentText(pct)})
	} else if used, ok := number(e, kube.AttrCPUUsageMilli); ok &&
		limited && 100*used/limit >= limitWarning {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceCPUAtLimit, Value: milliText(used)})
	}
	if len(out) == 0 {
		return nil
	}
	if limited {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceCPULimit,
			Value: strconv.Itoa(int(limit)) + "m"})
	}
	if peak, ok := number(e, kube.AttrCPUPeak24h); ok && peak > 0 {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceCPUPeak, Value: milliText(peak)})
	}
	return out
}
