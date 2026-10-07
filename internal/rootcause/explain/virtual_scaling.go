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
		Prior: 0.70,
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

// addMetricsBackends lets whatever explains a failing metrics Service
// (its crashing pods, through their Deployment) also explain the
// autoscalers that Service explains: the HPAs failed because the
// metrics API had no backend, and the backend is what broke. Without
// it the Deployment and the Service would be two incidents.
func (v *view) addMetricsBackends(cs *candidateSet) {
	for _, id := range sortedKeys(cs.byID) {
		service := cs.byID[id]
		if service.id.Kind != kube.KindService {
			continue
		}
		for _, cause := range coveringService(cs, service.id) {
			for _, effect := range service.direct() {
				if effect.Kind == kube.KindHPA {
					cs.add(cause, effect, coverage{derived: true,
						match: rowMatch{row: summaryRow},
						chain: []inventory.EntityID{cause, effect}})
				}
			}
		}
	}
}

// coveringService lists the candidates, other than the Service itself,
// that explain the Service.
func coveringService(
	cs *candidateSet, service inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, id := range sortedKeys(cs.byID) {
		if _, ok := cs.byID[id].covers[service]; ok && id != service {
			out = append(out, id)
		}
	}
	return out
}
