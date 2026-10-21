package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An endpoint named in a pod's error text is blamed only while nothing
// shows that it answers. Two facts kwatch itself holds show that it
// does: its own probe of the endpoint connects, and the same workload
// calls it from another node without a failure.

// endpointAnswers reports that the endpoint the failing effect names is
// not what breaks it: the probe reaches it now, or the workload's pods
// on other nodes call it fine while the failures stay on one node.
func (v *view) endpointAnswers(
	endpoint, effect inventory.EntityID,
) bool {
	return v.probePasses(endpoint) || v.failsOnOneNodeOnly(effect)
}

// probePasses reports whether kwatch's active probe of the endpoint
// connects. An endpoint kwatch does not probe has no answer either way.
func (v *view) probePasses(endpoint inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(endpoint)
	if !ok {
		return false
	}
	healthy, known := e.Attribute(kube.AttrHealthy)
	if !known {
		return false
	}
	ok, _ = healthy.Value.AsBool()
	return ok
}

// failsOnOneNodeOnly reports whether the effect's workload fails only
// on the effect's node: a sibling pod runs healthy on another node and
// none fails there. The same configuration reaches the same endpoint
// from the other nodes, so what differs is this node. The siblings are
// inputs of the walk, so a change in one of them solves it again.
func (v *view) failsOnOneNodeOnly(effect inventory.EntityID) bool {
	pod, ok := v.unitOf(effect)
	if !ok {
		return false
	}
	node, ok := v.nodeOf(pod)
	if !ok {
		return false
	}
	healthyElsewhere := false
	for _, sibling := range v.siblings(pod) {
		other, known := v.nodeOf(sibling)
		if !known || other == node {
			continue
		}
		v.gate(pod, sibling)
		if v.unitFailing(sibling) {
			return false
		}
		healthyElsewhere = healthyElsewhere || v.healthyPod(sibling)
	}
	return healthyElsewhere
}
