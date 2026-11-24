package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeEndedPods is the pseudo mode of a node whose kubelet ended a pod
// when the node shut down: the pod's own DisruptionTarget condition
// says TerminationByKubelet.
const ModeEndedPods detection.Mode = "EndedPods"

// shutdownState is ModeEndedPods for the node a failed pod was ended
// on by its kubelet, and nothing for any other entity.
func (v *view) shutdownState(id, effect inventory.EntityID) state {
	if id.Kind != kube.KindNode {
		return state{id: id}
	}
	pod, ok := v.unitOf(effect)
	if !ok {
		return state{id: id}
	}
	e, found := v.s.Model.Entity(pod)
	if !found {
		return state{id: id}
	}
	key := kube.ConditionKey("DisruptionTarget")
	if attrText(e, key) != "True" ||
		attrText(e, key+kube.AttrConditionReason) != "TerminationByKubelet" {
		return state{id: id}
	}
	if at, on := v.nodeOf(pod); !on || at != id {
		return state{id: id}
	}
	return state{id: id, modes: []modeHealth{{
		mode: ModeEndedPods, health: detection.Failing, pseudo: true}}}
}

// ModeKubeletSilent is the pseudo mode of a node whose kubelet has not
// reported on pods bound to it: they are Pending with no start time.
const ModeKubeletSilent detection.Mode = "KubeletSilent"

// kubeletSilentMin is how many bound pods must be silent for the node
// to be suspected: one pod waiting may be anything.
const kubeletSilentMin = 2

// silentState is ModeKubeletSilent for a node that several pods are
// bound to without the kubelet having reported a start time for any.
func (v *view) silentState(id, effect inventory.EntityID) state {
	if id.Kind != kube.KindNode || !v.kubeletSilent(effect, id) {
		return state{id: id}
	}
	silent := 0
	for _, pod := range v.dependents(id) {
		if v.kubeletSilent(pod, id) {
			silent++
		}
	}
	if silent < kubeletSilentMin {
		return state{id: id}
	}
	return state{id: id, modes: []modeHealth{{
		mode: ModeKubeletSilent, health: detection.Failing,
		pseudo: true}}}
}

// kubeletSilent reports a Pending pod bound to node whose kubelet has
// reported no start time.
func (v *view) kubeletSilent(pod, node inventory.EntityID) bool {
	unit, ok := v.unitOf(pod)
	if !ok {
		return false
	}
	e, found := v.s.Model.Entity(unit)
	if !found || attrText(e, kube.AttrPhase) != "Pending" {
		return false
	}
	if _, started := attrTime(e, kube.AttrStartTime); started {
		return false
	}
	at, on := v.nodeOf(unit)
	return on && at == node
}
