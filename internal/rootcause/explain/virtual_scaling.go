package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkScales is the link of a cause that sets the effect's replica
// count: an HPA and its target workload.
const LinkScales LinkType = "scales"

// SignalTimeout is a request that ran out of time: the symptom of a
// replica too busy to answer, unlike a refused connection, which says
// nothing listens.
const SignalTimeout Signal = "timeout-error"

// scalingRows cover autoscalers that cannot add the capacity the load
// asks for.
var scalingRows = []Row{
	{
		// An autoscaler at its ceiling while its metric wants more
		// replicas leaves the existing ones saturated: their probes
		// time out and they turn unready. The pods and nodes are
		// healthy; the limit is too low for the load.
		Name: "autoscaling-limit",
		Cause: Side{Kind: kube.KindHPA,
			Modes: []detection.Mode{detection.ModeScalingMaxedOut}},
		Link: LinkScales,
		Effect: Side{Kind: kube.KindPod, Modes: []detection.Mode{
			detection.ModeNotReady,
			detection.ModeProbe}, Signal: SignalTimeout},
		Prior: 0.65,
	},
	{
		// The same ceiling seen from the workload it scales: fewer
		// replicas ready than it asked for, often before its pods
		// have been unready long enough to fail on their own. Weaker
		// than the pod row: no error text says the pods are busy.
		Name: "autoscaling-limit-unavailable",
		Cause: Side{Kind: kube.KindHPA,
			Modes: []detection.Mode{detection.ModeScalingMaxedOut}},
		Link: LinkScales,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{detection.ModeUnavailable}},
		Prior: 0.55,
	},
}

// scalerHops lead from a workload to the autoscalers that scale it.
// Only an autoscaler with a finding is followed: a healthy one cannot
// be a cause, and most workloads have none. Every autoscaler is a gate,
// so the walk is redone when one starts failing.
func (v *view) scalerHops(id inventory.EntityID) []hop {
	var out []hop
	for _, hpa := range v.s.Model.Related(
		id, inventory.Scales, inventory.Incoming,
	) {
		v.gate(id, hpa)
		if len(v.s.Findings[hpa]) > 0 {
			out = append(out, hop{link: LinkScales, to: hpa})
		}
	}
	return out
}
