package kube

import (
	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/inventory"
)

// churnKinds hold links that end on their own all the time: a pod's or
// container's on every restart, a node's as pods come and go, an
// EndpointSlice's as pods turn ready, and those of ReplicaSets and Jobs,
// which a rollout or a run replaces whole.
var churnKinds = map[inventory.Kind]bool{
	KindPod: true, KindContainer: true, KindNode: true,
	KindEndpointSlice: true, KindReplicaSet: true, KindJob: true,
}

// usingKinds are the workloads whose uses of ConfigMaps, Secrets and
// claims are recorded.
var usingKinds = map[inventory.Kind]bool{
	KindDeployment: true, KindStatefulSet: true, KindDaemonSet: true,
	KindCronJob: true,
}

// usedKinds are what a workload's recorded uses point to.
var usedKinds = map[inventory.Kind]bool{
	KindConfigMap: true, KindSecret: true, KindPVC: true,
}

// selectorKinds select workloads by their pod template labels: a
// Service routes to them, a NetworkPolicy covers them.
var selectorKinds = map[inventory.Kind]inventory.RelationType{
	KindService:       inventory.RoutesTo,
	KindNetworkPolicy: inventory.Selects,
}

// linkObservation reports an observation that can end a recorded link:
// the relations of a long-lived object, or a selector's object.
func linkObservation(o inventory.Observation) bool {
	switch o.Kind {
	case inventory.Related:
		return !churnKinds[o.Entity.Kind]
	case inventory.Observed:
		_, selects := selectorKinds[o.Entity.Kind]
		return (selects || usingKinds[o.Entity.Kind]) &&
			o.Entity.Group == "" && o.Source == ObservationSource
	}
	return false
}

// linksOf reads the recorded links o can end, as the model holds them
// now.
func linksOf(r inventory.Reader, o inventory.Observation) []heldLink {
	if o.Kind == inventory.Observed {
		if usingKinds[o.Entity.Kind] {
			return workloadCalls(r, o.Entity)
		}
		return selectedWorkloads(r, o.Entity)
	}
	if churnKinds[o.Entity.Kind] {
		return nil
	}
	var out []heldLink
	for _, to := range r.Related(o.Entity, o.Relation, inventory.Outgoing) {
		if recordedLink(o.Entity, o.Relation, to) {
			out = append(out, heldLink{from: o.Entity, rel: o.Relation, to: to})
		}
	}
	return out
}

// recordedLink reports a relation worth recording when it ends.
func recordedLink(
	from inventory.EntityID, rel inventory.RelationType,
	to inventory.EntityID,
) bool {
	switch rel {
	case inventory.References, inventory.Mounts:
		return usingKinds[from.Kind] && from.Group == "" &&
			usedKinds[to.Kind] && to.Group == ""
	case inventory.RoutesTo, inventory.Serves:
		return to.Kind == KindService && to.Group == ""
	case inventory.Calls:
		return true
	}
	return false
}

// selectedWorkloads are the workloads of the selector object's
// namespace whose pod template labels its selector matches. An empty
// NetworkPolicy selector ("<none>") covers every workload; a Service
// without a selector routes to none.
func selectedWorkloads(r inventory.Reader, id inventory.EntityID) []heldLink {
	rel, ok := selectorKinds[id.Kind]
	attribute, present := r.Attribute(id, AttrSelector)
	if !ok || !present || id.Namespace == "" {
		return nil
	}
	text := attribute.Value.AsText()
	selector, ok := parseWorkloadSelector(text, id.Kind)
	if !ok {
		return nil
	}
	var out []heldLink
	for _, kind := range WorkloadKinds {
		templates := r.AttributeIn(kind, id.Namespace, AttrTemplateLabels)
		for _, workload := range r.EntitiesIn(kind, id.Namespace) {
			set, err := labels.ConvertSelectorToLabelsMap(
				templates[workload].AsText())
			if err == nil && len(set) > 0 && selector.Matches(set) {
				out = append(out, heldLink{from: id, rel: rel, to: workload})
			}
		}
	}
	return out
}

// parseWorkloadSelector reads a selector attribute. "<none>" selects
// everything for a NetworkPolicy and nothing for a Service.
func parseWorkloadSelector(
	text string, kind inventory.Kind,
) (labels.Selector, bool) {
	switch {
	case text == "":
		return nil, false
	case text == "<none>":
		return labels.Everything(), kind == KindNetworkPolicy
	}
	selector, err := labels.Parse(text)
	return selector, err == nil && !selector.Empty()
}
