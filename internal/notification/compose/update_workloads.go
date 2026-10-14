package compose

import (
	"github.com/abahmed/kwatch/internal/inventory"
)

// workloadsOf names the workloads that run the pods and containers in
// ids, once each and in order, as an announcement does: replicas of one
// Deployment are one Deployment, and a pod's name says nothing a reader
// can act on. An entity with no known workload keeps its own name.
func workloadsOf(
	f caseFacts, ids []inventory.EntityID,
) []inventory.EntityID {
	known := append([]inventory.EntityID{f.p.Root}, f.p.Impact...)
	seen := map[inventory.EntityID]bool{}
	var out []inventory.EntityID
	for _, id := range ids {
		if owner, ok := ownerIn(known, id); ok {
			id = owner
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
