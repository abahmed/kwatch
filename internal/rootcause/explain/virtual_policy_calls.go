package explain

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkDenies links a NetworkPolicy, as the cause, to the pod whose call
// it stops.
const LinkDenies LinkType = "denies"

// ModeBlocksCall is a policy that stops the effect's call to a Service.
// A finer mode names the call: "BlocksCall.postgres:5432".
const ModeBlocksCall detection.Mode = "BlocksCall"

// maxCallEdges bounds the dependencies checked for one failing pod.
const maxCallEdges = 8

// policyCallRows say that a policy created or changed recently, which
// stops a call its pod makes, explains the pod's failure.
var policyCallRows = []Row{{
	Name: "policy-blocks-call",
	Cause: Side{Kind: kube.KindNetworkPolicy,
		Modes: []detection.Mode{ModeBlocksCall}},
	Link: LinkDenies,
	Effect: Side{Kind: kube.KindPod, Modes: append(
		append([]detection.Mode(nil), callerFailures...),
		detection.ModeNotReady, detection.ModeProbe)},
	Prior: 0.8,
}}

// blockedCall is a dependency edge a policy stops.
type blockedCall struct {
	service inventory.EntityID
	port    int
}

// blockKey is one policy blocking one pod.
type blockKey struct{ policy, pod inventory.EntityID }

// callEdges are the Services the pod calls: the one its error names,
// then the ones its configuration names that exist.
func (v *view) callEdges(pod inventory.EntityID) []blockedCall {
	var out []blockedCall
	if call, ok := v.clusterCallOf(pod); ok {
		out = append(out, blockedCall{call.service, call.port})
	}
	if entity, ok := v.s.Model.Entity(pod); ok {
		for _, ref := range kube.ServiceCalls(entity) {
			if v.s.Model.Exists(ref.Service) {
				out = append(out, blockedCall{ref.Service, ref.Port})
			}
		}
	}
	if len(out) > maxCallEdges {
		out = out[:maxCallEdges]
	}
	return out
}

// policyCallHops lead from a failing pod to the policies that stop one
// of its calls. The namespaces gate the hop: a policy created later
// marks its namespace as moved.
func (v *view) policyCallHops(pod inventory.EntityID) []hop {
	if !v.unitGate(pod) {
		return nil
	}
	if v.blocks == nil {
		v.blocks = map[blockKey]blockedCall{}
	}
	var out []hop
	for _, edge := range v.callEdges(pod) {
		v.gate(pod, inventory.CoreID(kube.KindNamespace, "", pod.Namespace))
		v.gate(pod, inventory.CoreID(kube.KindNamespace, "",
			edge.service.Namespace))
		block, blocked := kube.CallBlocked(v.s.Model, pod, edge.service,
			edge.port)
		if !blocked {
			continue
		}
		for _, policy := range block.Policies {
			key := blockKey{policy, pod}
			if _, seen := v.blocks[key]; !seen {
				v.blocks[key] = edge
				out = append(out, hop{link: LinkDenies, to: policy})
			}
		}
	}
	return out
}

// blockModes is the pseudo mode of a policy that blocks the effect's
// call and changed inside the causal window: a policy that has always
// been there is not what broke it.
func (v *view) blockModes(
	policy, effect inventory.EntityID, link LinkType,
) []modeHealth {
	pod, ok := v.unitOf(effect)
	if !ok || link != LinkDenies || len(v.changesOf(policy)) == 0 {
		return nil
	}
	edge, ok := v.blocks[blockKey{policy, pod}]
	if !ok {
		return nil
	}
	return []modeHealth{{health: detection.Failing, pseudo: true,
		mode: ModeBlocksCall + detection.Mode("."+edge.service.Name+
			callPort(edge))}}
}

func callPort(edge blockedCall) string {
	if edge.port == 0 {
		return ""
	}
	return ":" + strconv.Itoa(edge.port)
}
