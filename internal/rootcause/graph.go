package rootcause

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// PodOf returns the pod an entity belongs to: the pod itself, or the pod a
// container is part of.
func PodOf(model inventory.Reader, id inventory.EntityID) (
	inventory.EntityID, bool,
) {
	switch id.Kind {
	case kube.KindPod:
		return id, true
	case kube.KindContainer:
		pods := model.Related(id, inventory.PartOf, inventory.Outgoing)
		if len(pods) > 0 {
			return pods[0], true
		}
	}
	return inventory.EntityID{}, false
}

// OwnerChain walks owned-by from id to the top controller, id excluded.
func OwnerChain(
	model inventory.Reader, id inventory.EntityID,
) []inventory.EntityID {
	var chain []inventory.EntityID
	seen := map[inventory.EntityID]bool{id: true}
	for current := id; ; {
		owner, ok := inventory.Owner(model, current)
		// Static pods are "owned" by their Node; the node is where they
		// run, not a controller that stamps them out.
		if !ok || seen[owner] || owner.Kind == kube.KindNode {
			return chain
		}
		current = owner
		seen[current] = true
		chain = append(chain, current)
	}
}

// TopOwner is the controller that ultimately owns id, or id itself.
func TopOwner(
	model inventory.Reader, id inventory.EntityID,
) inventory.EntityID {
	chain := OwnerChain(model, id)
	if len(chain) == 0 {
		return id
	}
	return chain[len(chain)-1]
}

// OwnedPods lists every pod a controller owns, directly or through
// intermediate controllers such as ReplicaSets.
func OwnedPods(
	model inventory.Reader, owner inventory.EntityID,
) []inventory.EntityID {
	return kube.OwnedPods(model, owner)
}
