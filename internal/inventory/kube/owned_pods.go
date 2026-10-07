package kube

import "github.com/abahmed/kwatch/internal/inventory"

// OwnedPods lists every pod a controller owns, directly or through
// intermediate controllers such as ReplicaSets.
func OwnedPods(
	model inventory.Reader, owner inventory.EntityID,
) []inventory.EntityID {
	if owner.Kind == KindPod {
		return []inventory.EntityID{owner}
	}
	var out []inventory.EntityID
	collectPods(model, owner, map[inventory.EntityID]bool{}, &out)
	return out
}

func collectPods(
	model inventory.Reader, owner inventory.EntityID,
	seen map[inventory.EntityID]bool, out *[]inventory.EntityID,
) {
	if seen[owner] {
		return
	}
	seen[owner] = true
	for _, child := range model.Related(
		owner, inventory.OwnedBy, inventory.Incoming,
	) {
		if child.Kind == KindPod {
			*out = append(*out, child)
			continue
		}
		collectPods(model, child, seen, out)
	}
}
