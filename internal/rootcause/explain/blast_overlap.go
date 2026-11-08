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
	total := v.failingPodsIn(c.id.Namespace)
	if total < BlastOverlapMinPods {
		return outcome{}
	}
	share := float64(len(pods)) / float64(total)
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
		if pods := v.failingPodsOf(c.others()); len(pods) > 0 {
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

// podsBehind lists the pods a workload effect owns.
func (v *view) podsBehind(effect inventory.EntityID) []inventory.EntityID {
	var pods []inventory.EntityID
	for _, id := range v.s.Model.Related(effect, inventory.OwnedBy,
		inventory.Incoming) {
		if id.Kind == kube.KindPod {
			pods = append(pods, id)
		}
	}
	return pods
}

// failingPodsIn counts the distinct pods of a namespace with an
// unhealthy finding of their own or of one of their containers.
func (v *view) failingPodsIn(namespace string) int {
	pods := map[inventory.EntityID]bool{}
	for id, findings := range v.s.Findings {
		if id.Namespace != namespace {
			continue
		}
		pod, ok := rootcause.PodOf(v.s.Model, id)
		if !ok {
			continue
		}
		for _, f := range findings {
			if unhealthy(f) && !f.Symptom {
				pods[pod] = true
			}
		}
	}
	return len(pods)
}
