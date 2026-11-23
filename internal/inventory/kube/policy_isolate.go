package kube

import (
	"sort"

	"github.com/abahmed/kwatch/internal/inventory"
)

// IsolatingPolicies are the NetworkPolicies that cut every pod behind
// the Service off from all ingress: each backend pod is selected for
// ingress and none of the policies selecting it has an ingress rule.
// Nothing can reach the pods then, whoever the caller is, so the check
// needs no caller. It is empty when the Service has no pods the model
// knows or when any pod is open to some traffic.
func IsolatingPolicies(
	r inventory.Reader, service inventory.EntityID,
) []inventory.EntityID {
	backends := backendPods(r, service)
	seen := map[inventory.EntityID]bool{}
	for _, pod := range backends {
		selecting := policiesSelecting(r, pod, false)
		if len(selecting) == 0 {
			return nil
		}
		for _, p := range selecting {
			if len(p.rules.In) > 0 {
				return nil
			}
			seen[p.id] = true
		}
	}
	out := make([]inventory.EntityID, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		return inventory.CompareIDs(out[i], out[j]) < 0
	})
	return out
}
