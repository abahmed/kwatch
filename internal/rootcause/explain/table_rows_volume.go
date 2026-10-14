package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkHoldsVolume says the cause still holds the volume the effect pod
// waits for (an old pod that mounts the claim, or the node it is
// attached to).
const LinkHoldsVolume LinkType = "holds-volume"

// LinkProvisions says the cause is the workload that provisions the
// volumes of the effect claim's StorageClass.
const LinkProvisions LinkType = "provisions"

// Pseudo modes of the volume chain.
const (
	// ModeHoldsVolume is a pod or node that still holds the volume a
	// pod's multi-attach error names.
	ModeHoldsVolume detection.Mode = "HoldsVolume"
	// ModeProvisionerDown is a workload serving a provisioner that has
	// no ready pod.
	ModeProvisionerDown detection.Mode = "ProvisionerDown"
	// ModeNoStorageClass is a claim that names no class when the
	// cluster has no default class and no static volume to give it.
	ModeNoStorageClass detection.Mode = "Config.NoStorageClass"
)

// volumeRows explain a pod or claim that waits for storage.
var volumeRows = []Row{
	{
		// The attach-detach controller refuses a ReadWriteOnce volume
		// while another pod still uses it on another node: the old pod
		// is what the new one waits for.
		Name: "volume-held-by-pod",
		Cause: Side{Kind: kube.KindPod,
			Modes: []detection.Mode{ModeHoldsVolume}},
		Link:   LinkHoldsVolume,
		Effect: podSide(detection.ModeVolumeMultiAttach),
		Prior:  0.8,
	},
	{
		// No pod holds the volume any more but its attachment is still
		// on the old node.
		Name: "volume-held-by-node",
		Cause: Side{Kind: kube.KindNode,
			Modes: []detection.Mode{ModeHoldsVolume}},
		Link:   LinkHoldsVolume,
		Effect: podSide(detection.ModeVolumeMultiAttach),
		Prior:  0.7,
	},
	{
		Name: "provisioner-down",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeProvisionerDown}},
		Link:   LinkProvisions,
		Effect: unboundClaimSide(),
		Prior:  0.8,
	},
	{
		// The same provisioner, seen from the pod that waits for the
		// claim: the claim is only the way from the pod to it.
		Name: "provisioner-down-pending",
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeProvisionerDown}},
		Link: LinkProvisions,
		Effect: podSide(detection.ModePending, detection.ModeUnschedulable,
			detection.ModeCreating),
		Prior: 0.8,
	},
	{
		Name: "claim-no-storage-class",
		Cause: Side{Kind: kube.KindPVC,
			Modes: []detection.Mode{ModeNoStorageClass}},
		Link:   LinkSelf,
		Effect: unboundClaimSide(),
		Prior:  0.8, Inside: true,
	},
}

// unboundClaimSide is a claim that cannot get a volume.
func unboundClaimSide() Side {
	return Side{Kind: kube.KindPVC, Modes: []detection.Mode{
		detection.ModeClaimFailed, detection.ModeVolumeProvisioningFailed,
		detection.ModePending}}
}

// holdHops lead from a pod with a multi-attach error to what still holds
// its volume: the pods on other nodes that mount the same claim, or else
// the nodes the volume is attached to.
func (v *view) holdHops(pod inventory.EntityID) []hop {
	if pod.Kind != kube.KindPod || !v.hasMode(pod,
		[]detection.Mode{detection.ModeVolumeMultiAttach}) {
		return nil
	}
	var ids []inventory.EntityID
	for _, holder := range kube.VolumeHolders(v.s.Model, pod) {
		if holder.Pod.IsZero() {
			ids = append(ids, holder.Node)
		} else {
			ids = append(ids, holder.Pod)
		}
	}
	return hopsTo(LinkHoldsVolume, sampleIDs(ids)...)
}

// heldModes is ModeHoldsVolume for a pod or node that holds the volume
// of the effect pod's multi-attach error.
func (v *view) heldModes(id, effect inventory.EntityID) []modeHealth {
	pod, ok := v.unitOf(effect)
	if !ok {
		return nil
	}
	for _, hop := range v.holdHops(pod) {
		if hop.to == id {
			return pseudoFailing(ModeHoldsVolume)
		}
	}
	return nil
}
