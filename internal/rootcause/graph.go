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

// ownerChain walks owned-by from id to the top controller, id excluded.
func ownerChain(
	model inventory.Reader, id inventory.EntityID,
) []inventory.EntityID {
	var chain []inventory.EntityID
	seen := map[inventory.EntityID]bool{id: true}
	for current := id; ; {
		owners := model.Related(
			current, inventory.OwnedBy, inventory.Outgoing)
		// Static pods are "owned" by their Node; the node is where they
		// run, not a controller that stamps them out.
		if len(owners) == 0 || seen[owners[0]] ||
			owners[0].Kind == kube.KindNode {
			return chain
		}
		current = owners[0]
		seen[current] = true
		chain = append(chain, current)
	}
}

// TopOwner is the controller that ultimately owns id, or id itself.
func TopOwner(
	model inventory.Reader, id inventory.EntityID,
) inventory.EntityID {
	chain := ownerChain(model, id)
	if len(chain) == 0 {
		return id
	}
	return chain[len(chain)-1]
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
		if child.Kind == kube.KindPod {
			*out = append(*out, child)
			continue
		}
		collectPods(model, child, seen, out)
	}
}

// OwnedPods lists every pod a controller owns, directly or through
// intermediate controllers such as ReplicaSets.
func OwnedPods(
	model inventory.Reader, owner inventory.EntityID,
) []inventory.EntityID {
	if owner.Kind == kube.KindPod {
		return []inventory.EntityID{owner}
	}
	var out []inventory.EntityID
	collectPods(model, owner, map[inventory.EntityID]bool{}, &out)
	return out
}
