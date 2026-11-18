package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// endedPolicyHops lead from a pod to the network policies of its
// namespace that stopped covering its workload inside the change window
// (a change that records the end of their selects link, see
// kube.LinkJournal). The model no longer links them: an allow policy
// that stopped selecting the workload is what cut its connections
// under a default deny, and the policy-restricts row judges it like a
// policy that selects the pod.
func (v *view) endedPolicyHops(pod inventory.EntityID) []hop {
	workload := rootcause.TopOwner(v.s.Model, pod)
	var out []hop
	for _, policy := range v.policiesIn(pod.Namespace) {
		if v.selected(policy)[pod] {
			continue
		}
		for _, change := range v.changesOf(policy) {
			rel, target, ok := change.RemovedLink()
			if ok && rel == inventory.Selects && target == workload {
				out = append(out, hop{link: LinkRestricts, to: policy})
				break
			}
		}
	}
	return out
}
