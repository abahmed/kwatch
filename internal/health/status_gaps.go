package health

import (
	"sort"
	"strconv"

	"github.com/abahmed/kwatch/internal/status"
)

// gapReasons word the coverage reason codes.
var gapReasons = map[string]string{
	CoveragePermissionDenied: "forbidden, kwatch cannot read it",
	CoverageAPIUnavailable:   "the API does not serve it",
	CoverageSyncTimeout:      "did not finish syncing",
	CoverageBudgetExceeded:   "over the watch budget",
	CoverageDisabledByConfig: "turned off in the configuration",
	CoverageObjectCapReached: "partly watched, more objects than the cap",
}

// gapComponents are the components whose failure leaves part of the
// cluster unseen.
var gapComponents = map[string]string{
	"kubelet-stats": "node stats from the kubelets",
	"rbac":          "permissions kwatch needs",
}

// coverageGaps lists what kwatch cannot see: kinds it does not watch,
// kubelets it cannot reach and permissions it lacks. It reuses the
// coverage and component state /health serves.
func (h *HealthServer) coverageGaps() []status.Gap {
	var out []status.Gap
	if c := h.Coverage(); c != nil {
		for _, k := range c.UnavailableKinds {
			out = append(out, status.Gap{What: k.Kind + " objects",
				Reason: gapReasons[k.Reason]})
		}
		if more := reasonTotal(c) - len(c.UnavailableKinds); more > 0 {
			out = append(out, status.Gap{
				What:   strconv.Itoa(more) + " more kinds",
				Reason: "listed by reason on /health"})
		}
		if !c.Complete {
			out = append(out, status.Gap{What: "API discovery",
				Reason: "partial, some kinds may be missing"})
		}
	}
	out = append(out, h.componentGaps()...)
	if len(out) > status.MaxGaps {
		out = out[:status.MaxGaps]
	}
	return out
}

// reasonTotal counts every kind with a reason, listed or not.
func reasonTotal(c *CoverageSummary) int {
	total := 0
	for _, n := range c.Reasons {
		total += n
	}
	return total
}

func (h *HealthServer) componentGaps() []status.Gap {
	degraded := h.ComponentErrors()
	names := make([]string, 0, len(degraded))
	for name := range degraded {
		if _, ok := gapComponents[name]; ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var out []status.Gap
	for _, name := range names {
		out = append(out, status.Gap{What: gapComponents[name],
			Reason: "unreachable or degraded (" + degraded[name] + ")"})
	}
	return out
}
