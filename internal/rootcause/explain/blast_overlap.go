package explain

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreBlastOverlap ranks changed candidates by how much of the failure
// is downstream of them: the failing pods that depend on the candidate,
// over all the failing pods in its namespace. Two ConfigMaps edited in
// the same minute are told apart by the pods that read each. Only a
// candidate that changed inside the causal window is judged; one that
// did not change is ranked by the other scorers.
func scoreBlastOverlap(v *view, c *candidate) outcome {
	pods := v.blast[c.id]
	if len(pods) == 0 || !v.hasChangedRival(c.id) {
		return outcome{}
	}
	total := v.failingPodsAmong(namespacesOf(pods))
	if total < BlastOverlapMinPods {
		return outcome{}
	}
	share := min(float64(len(pods))/float64(total), 1)
	out := outcome{code: rootcause.ProofBlastOverlap,
		count: len(pods), total: total,
		text: fmt.Sprintf("%d of %d failing pods are downstream of it",
			len(pods), total)}
	switch {
	case share >= BlastOverlapMin:
		out.weight = BlastOverlapWeight *
			(share - BlastOverlapMin) / (1 - BlastOverlapMin)
	case share < BlastOverlapLow:
		out.weight = -BlastOverlapPenalty
	default:
		return outcome{}
	}
	return out
}

// noteBlast records, for every candidate changed inside the causal
// window, the failing pods downstream of it.
func (v *view) noteBlast(cs *candidateSet) {
	v.blast = map[inventory.EntityID]map[inventory.EntityID]bool{}
	for id, c := range cs.byID {
		if c.isSelf() || !v.changedInWindow(id) {
			continue
		}
		pods := v.failingOnly(v.failingPodsOf(c.others()))
		if len(pods) > 0 {
			v.blast[id] = pods
		}
	}
}

// hasChangedRival reports another changed candidate that shares a
// failing pod with id: only then is there a choice between changes for
// the overlap to settle. A lone change is ranked by the other scorers.
func (v *view) hasChangedRival(id inventory.EntityID) bool {
	for other, pods := range v.blast {
		if other == id {
			continue
		}
		for pod := range pods {
			if v.blast[id][pod] {
				return true
			}
		}
	}
	return false
}

// changedInWindow reports a candidate with a change in the causal
// window other than a replica count.
func (v *view) changedInWindow(id inventory.EntityID) bool {
	for _, change := range v.changesOf(id) {
		if changeMode(change) != ModeScaled {
			return true
		}
	}
	return false
}

// failingPodsOf lists the distinct pods among effects.
func (v *view) failingPodsOf(
	effects []inventory.EntityID,
) map[inventory.EntityID]bool {
	pods := map[inventory.EntityID]bool{}
	for _, effect := range effects {
		if pod, ok := rootcause.PodOf(v.s.Model, effect); ok {
			pods[pod] = true
		}
		for _, pod := range v.podsBehind(effect) {
			pods[pod] = true
		}
	}
	return pods
}

// podsBehind lists the pods a workload effect owns, directly or through
// its ReplicaSets or Jobs.
func (v *view) podsBehind(effect inventory.EntityID) []inventory.EntityID {
	var pods []inventory.EntityID
	level := []inventory.EntityID{effect}
	for depth := 0; depth < 2 && len(level) > 0; depth++ {
		var next []inventory.EntityID
		for _, owner := range level {
			for _, id := range v.s.Model.Related(owner, inventory.OwnedBy,
				inventory.Incoming) {
				if id.Kind == kube.KindPod {
					pods = append(pods, id)
				} else {
					next = append(next, id)
				}
			}
		}
		level = next
	}
	return pods
}

// failingPodSet is every pod with an unhealthy finding of its own or of
// one of its containers, symptoms left out: the one rule both sides of
// the overlap share.
func (v *view) failingPodSet() map[inventory.EntityID]bool {
	if v.failingPods != nil {
		return v.failingPods
	}
	v.failingPods = map[inventory.EntityID]bool{}
	for id, findings := range v.s.Findings {
		pod, ok := rootcause.PodOf(v.s.Model, id)
		if !ok {
			continue
		}
		for _, f := range findings {
			if unhealthy(f) && !f.Symptom {
				v.failingPods[pod] = true
			}
		}
	}
	return v.failingPods
}

// failingOnly keeps the pods that are failing by failingPodSet's rule.
func (v *view) failingOnly(
	pods map[inventory.EntityID]bool,
) map[inventory.EntityID]bool {
	out := map[inventory.EntityID]bool{}
	for pod := range pods {
		if v.failingPodSet()[pod] {
			out[pod] = true
		}
	}
	return out
}

// namespacesOf lists the namespaces of pods, so a cluster-scoped
// candidate is judged over the namespaces it reaches, like a namespaced
// one.
func namespacesOf(pods map[inventory.EntityID]bool) map[string]bool {
	out := map[string]bool{}
	for pod := range pods {
		out[pod.Namespace] = true
	}
	return out
}

// failingPodsAmong counts the failing pods in the namespaces.
func (v *view) failingPodsAmong(namespaces map[string]bool) int {
	n := 0
	for pod := range v.failingPodSet() {
		if namespaces[pod.Namespace] {
			n++
		}
	}
	return n
}
