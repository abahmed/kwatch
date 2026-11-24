package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// cordonTaint is the taint the cordon itself sets; it says nothing
// beyond the cordon, so it does not count against the node.
const cordonTaint = "node.kubernetes.io/unschedulable"

// cordonWouldFit reports whether the pod would fit the cordoned node
// were the cordon lifted: the node's free CPU and memory (allocatable
// minus what its running pods request) cover the pod's requests, its
// taints are tolerated and its labels meet the pod's node selector and
// required node affinity. Only then did the cordon, not the node
// itself, leave the pod without a place. A node whose capacity kwatch
// does not know is not judged to fit.
func (v *view) cordonWouldFit(
	pod inventory.Entity, node inventory.Entity,
) bool {
	spec := kube.ParseSchedulingSpec(pod)
	return v.cordonHasRoom(pod.ID, node) &&
		cordonTolerated(spec, node) && v.cordonLabelsMatch(spec, node)
}

// cordonHasRoom compares the pod's CPU and memory requests with what
// the node has left of each.
func (v *view) cordonHasRoom(
	pod inventory.EntityID, node inventory.Entity,
) bool {
	for _, r := range []struct{ request, allocatable string }{
		{kube.AttrCPUReq, kube.AttrCPUAllocatable},
		{kube.AttrMemoryReq, kube.AttrMemoryAllocatable},
	} {
		total, known := attrNumber(node, r.allocatable)
		if !known {
			return false
		}
		used := 0.0
		for _, other := range v.s.Model.Related(node.ID,
			inventory.RunsOn, inventory.Incoming) {
			if other != pod && !v.podFinished(other) {
				used += v.podRequest(other, r.request)
			}
		}
		if total-used < v.podRequest(pod, r.request) {
			return false
		}
	}
	return true
}

// podFinished reports a pod that has ended and holds no resources.
func (v *view) podFinished(pod inventory.EntityID) bool {
	e, ok := v.s.Model.Entity(pod)
	if !ok {
		return true
	}
	phase := attrText(e, kube.AttrPhase)
	return phase == "Succeeded" || phase == "Failed"
}

// podRequest is what the scheduler reserves of one resource for a pod:
// the sum over its containers, or its largest init container when that
// is bigger.
func (v *view) podRequest(pod inventory.EntityID, attr string) float64 {
	sum, initMax := 0.0, 0.0
	for _, id := range v.s.Model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		c, ok := v.s.Model.Entity(id)
		if !ok || id.Kind != kube.KindContainer {
			continue
		}
		ask, _ := attrNumber(c, attr)
		if attrBool(c, kube.AttrInit) {
			initMax = max(initMax, ask)
		} else {
			sum += ask
		}
	}
	return max(sum, initMax)
}

// cordonTolerated reports whether the pod tolerates every taint of the
// node that keeps pods off, the cordon's own taint aside.
func cordonTolerated(spec kube.SchedulingSpec, node inventory.Entity) bool {
	for _, taint := range kube.ParseTaints(attrText(node, kube.AttrTaints)) {
		if taint.Key == cordonTaint || taint.Effect == "PreferNoSchedule" {
			continue
		}
		tolerated := false
		for _, t := range spec.Tolerations {
			tolerated = tolerated || t.Tolerates(taint)
		}
		if !tolerated {
			return false
		}
	}
	return true
}

// cordonLabelsMatch reports whether the node's labels meet the pod's
// node selector and one of its required node affinity terms. A node
// whose labels were too many to keep is not judged to match.
func (v *view) cordonLabelsMatch(
	spec kube.SchedulingSpec, node inventory.Entity,
) bool {
	if len(spec.Selector) == 0 && len(spec.NodeTerms) == 0 {
		return true
	}
	zone := ""
	for _, id := range v.s.Model.Related(node.ID, inventory.PartOf,
		inventory.Outgoing) {
		if id.Kind == kube.KindZone {
			zone = id.Name
		}
	}
	labels, ok := kube.NodeLabels(node, zone)
	if !ok {
		return false
	}
	for key, want := range spec.Selector {
		if labels[key] != want {
			return false
		}
	}
	if len(spec.NodeTerms) == 0 {
		return true
	}
	for _, term := range spec.NodeTerms {
		if kube.MatchesAll(term, labels) {
			return true
		}
	}
	return false
}
