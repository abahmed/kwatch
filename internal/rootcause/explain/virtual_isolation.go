package explain

import (
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeIsolatesBackend is a policy, created or changed recently, that
// cuts every pod behind the Service an admission webhook or an
// APIService calls off from all ingress.
const ModeIsolatesBackend detection.Mode = "IsolatesBackend"

// RulePolicyIsolatesBackend names the rows below.
const RulePolicyIsolatesBackend = "policy-isolates-backend"

// isolationRows say that such a policy explains what the API server
// cannot do without the backend it calls: creates a webhook rejects, a
// metrics API that does not answer, and the autoscalers that read it.
var isolationRows = []Row{
	{
		Name: RulePolicyIsolatesBackend,
		Cause: Side{Kind: kube.KindNetworkPolicy,
			Modes: []detection.Mode{ModeIsolatesBackend}},
		Link: LinkDenies,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes:  createModes,
			Signal: SignalWebhook},
		Prior: 0.85,
	},
	{
		Name: RulePolicyIsolatesBackend,
		Cause: Side{Kind: kube.KindNetworkPolicy,
			Modes: []detection.Mode{ModeIsolatesBackend}},
		Link: LinkDenies,
		Effect: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{detection.ModeAPIServiceUnavailable,
				detection.ModeScalingNoMetrics}},
		Prior: 0.85,
	},
}

// calledByAPIServer lists what the API server calls on behalf of the
// effect: the admission webhooks for a controller that cannot create,
// the metrics APIServices for an autoscaler, the entity itself when it
// is one of them.
func (v *view) calledByAPIServer(effect inventory.EntityID) []inventory.EntityID {
	switch {
	case isAdmissionKind(effect.Kind) || effect.Kind == kube.KindAPIService:
		return []inventory.EntityID{effect}
	case effect.Kind == kube.KindHPA:
		var out []inventory.EntityID
		for _, api := range v.s.Model.Entities(kube.KindAPIService) {
			if strings.Contains(api.Name, metricsAPIMarker) {
				out = append(out, api)
			}
		}
		return out
	case v.hasMode(effect, createModes):
		var out []inventory.EntityID
		for _, kind := range admitKinds {
			out = append(out, v.s.Model.Entities(kind)...)
		}
		return out
	}
	return nil
}

// isolatedBackends are the policies that isolate the pods of the
// Services the effect's API server calls reach.
func (v *view) isolatedBackends(
	effect inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, called := range v.calledByAPIServer(effect) {
		for _, service := range v.s.Model.Related(
			called, inventory.Serves, inventory.Outgoing) {
			v.gate(effect, inventory.CoreID(kube.KindNamespace, "",
				service.Namespace))
			for _, policy := range kube.IsolatingPolicies(
				v.s.Model, service) {
				if !slices.Contains(out, policy) {
					out = append(out, policy)
				}
			}
		}
	}
	return out
}

// isolationHops lead from a failing effect to the policies that isolate
// the backends its API server calls reach.
func (v *view) isolationHops(id inventory.EntityID) []hop {
	if !v.failing(id) {
		return nil
	}
	return hopsTo(LinkDenies, v.isolatedBackends(id)...)
}

// isolationModes is ModeIsolatesBackend for a policy that isolates such
// a backend and changed inside the causal window.
func (v *view) isolationModes(
	policy, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkDenies || len(v.changesOf(policy)) == 0 ||
		!slices.Contains(v.isolatedBackends(effect), policy) {
		return nil
	}
	return pseudoFailing(ModeIsolatesBackend)
}
