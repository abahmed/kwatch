package kube

import "github.com/abahmed/kwatch/internal/inventory"

// VolumeHolder is what still holds a volume a pod waits for: another
// pod that mounts the same claim from another node, or, when no such
// pod is left, only the node the volume is still attached to.
type VolumeHolder struct {
	// Pod is the pod using the volume; zero for a bare attachment.
	Pod inventory.EntityID
	// Node is the node the volume is attached to.
	Node inventory.EntityID
	// Terminating is a Pod that has been asked to stop.
	Terminating bool
}

// VolumeHolders lists what holds the claims of pod on a node other than
// its own. Pods that mount a claim come first, sorted; only when there
// are none do the VolumeAttachments of the claims' volumes name nodes.
func VolumeHolders(
	model inventory.Reader, pod inventory.EntityID,
) []VolumeHolder {
	own := model.Related(pod, inventory.RunsOn, inventory.Outgoing)
	claims := model.Related(pod, inventory.Mounts, inventory.Outgoing)
	if holders := holdingPods(model, pod, claims, own); len(holders) > 0 {
		return holders
	}
	return holdingNodes(model, claims, own)
}

func holdingPods(
	model inventory.Reader, pod inventory.EntityID,
	claims, own []inventory.EntityID,
) []VolumeHolder {
	var out []VolumeHolder
	for _, claim := range claims {
		for _, other := range model.Related(
			claim, inventory.Mounts, inventory.Incoming,
		) {
			if other == pod {
				continue
			}
			if holder, ok := holderOf(model, other, own); ok {
				out = append(out, holder)
			}
		}
	}
	return out
}

// holderOf is pod as a holder of the volume, when it runs on a node
// other than own and is in a state that holds a volume.
func holderOf(
	model inventory.Reader, pod inventory.EntityID, own []inventory.EntityID,
) (VolumeHolder, bool) {
	nodes := model.Related(pod, inventory.RunsOn, inventory.Outgoing)
	if len(nodes) == 0 || sharesNode(nodes, own) {
		return VolumeHolder{}, false
	}
	holder := VolumeHolder{Pod: pod, Node: nodes[0]}
	if e, ok := model.Entity(pod); ok {
		if !mountsVolume(e) {
			return VolumeHolder{}, false
		}
		if deleting, ok := e.Attribute(AttrDeleting); ok {
			holder.Terminating, _ = deleting.Value.AsBool()
		}
	}
	return holder, true
}

// mountsVolume reports a pod that can hold a volume: one that has
// finished (Succeeded, Failed) has let go of it, and one still Pending
// is itself waiting for the volume. A pod with no known phase counts.
func mountsVolume(pod inventory.Entity) bool {
	if phase, ok := pod.Attribute(AttrPhase); ok {
		switch phase.Value.AsText() {
		case "Succeeded", "Failed", "Pending":
			return false
		}
	}
	return true
}

func holdingNodes(
	model inventory.Reader, claims, own []inventory.EntityID,
) []VolumeHolder {
	var out []VolumeHolder
	for _, claim := range claims {
		for _, volume := range model.Related(
			claim, inventory.References, inventory.Outgoing,
		) {
			for _, node := range attachedNodes(model, volume) {
				if !sharesNode([]inventory.EntityID{node}, own) {
					out = append(out, VolumeHolder{Node: node})
				}
			}
		}
	}
	return out
}

// attachedNodes are the nodes the VolumeAttachments of volume name.
func attachedNodes(
	model inventory.Reader, volume inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, attach := range model.Related(
		volume, inventory.References, inventory.Incoming,
	) {
		if attach.Kind == KindVolumeAttachment {
			out = append(out, model.Related(attach, inventory.RunsOn,
				inventory.Outgoing)...)
		}
	}
	return out
}

func sharesNode(a, b []inventory.EntityID) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
