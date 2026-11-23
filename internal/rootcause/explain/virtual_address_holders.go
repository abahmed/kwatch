package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// LinkHoldsAddress says the cause is what keeps pods that hold pod
// addresses on a node from going away: the effect pod there cannot get
// an address.
const LinkHoldsAddress LinkType = "holds-address"

// RuleFinalizerHoldsAddresses names the rows below.
const RuleFinalizerHoldsAddresses = "finalizer-holds-addresses"

// MinAddressHolders is how many pods stuck Terminating on one node make
// their finalizer what the node's address pool waits for: one leftover
// pod frees one address, several are a pool in use.
const MinAddressHolders = 2

// addressHolderRows say that the finalizer (or the controller that
// should remove it) holding several pods Terminating on a node explains
// the pods on that node that cannot get an address.
var addressHolderRows = []Row{
	{
		Name: RuleFinalizerHoldsAddresses,
		Cause: Side{Kind: KindFinalizer,
			Modes: []detection.Mode{ModeFinalizerUnhandled}},
		Link: LinkHoldsAddress,
		Effect: Side{Kind: kube.KindPod,
			Modes: []detection.Mode{detection.ModeNetworkIPExhausted}},
		Prior: 0.7,
	},
	{
		Name: RuleFinalizerHoldsAddresses,
		Cause: Side{Group: AnyGroup, Kind: AnyKind,
			Modes: []detection.Mode{ModeHandlerStopped}},
		Link: LinkHoldsAddress,
		Effect: Side{Kind: kube.KindPod,
			Modes: []detection.Mode{detection.ModeNetworkIPExhausted}},
		Prior: 0.7,
	},
}

// addressHolders are the pods stuck Terminating, held by a finalizer,
// on the node of a pod that cannot get an address.
func (v *view) addressHolders(pod inventory.EntityID) []inventory.EntityID {
	if pod.Kind != kube.KindPod || !v.hasMode(pod,
		[]detection.Mode{detection.ModeNetworkIPExhausted}) {
		return nil
	}
	node, ok := v.nodeOf(pod)
	if !ok {
		return nil
	}
	var out []inventory.EntityID
	for _, other := range v.s.Model.Related(
		node, inventory.RunsOn, inventory.Incoming) {
		if other != pod && len(v.finalizersOf(other)) > 0 {
			out = append(out, other)
		}
	}
	if len(out) < MinAddressHolders {
		return nil
	}
	return sampleIDs(out)
}

// addressHolderHops lead from a pod that cannot get an address to what
// holds the finalizers of the pods that keep addresses on its node.
func (v *view) addressHolderHops(pod inventory.EntityID) []hop {
	var out []hop
	for _, held := range v.addressHolders(pod) {
		for _, h := range v.finalizerHops(held) {
			if !containsID(hopTargets(out), h.to) {
				out = append(out, hop{link: LinkHoldsAddress, to: h.to})
			}
		}
	}
	return out
}

func hopTargets(hops []hop) []inventory.EntityID {
	out := make([]inventory.EntityID, 0, len(hops))
	for _, h := range hops {
		out = append(out, h.to)
	}
	return out
}

// addressHolderModes are the modes of the finalizer or its controller,
// as they are for the stuck pods it holds.
func (v *view) addressHolderModes(
	id, effect inventory.EntityID, link LinkType,
) ([]modeHealth, bool) {
	if link != LinkHoldsAddress {
		return nil, false
	}
	pod, ok := v.unitOf(effect)
	if !ok {
		return nil, false
	}
	for _, held := range v.addressHolders(pod) {
		if modes, ok := v.finalizerModes(id, held, LinkFinalizes); ok {
			return modes, true
		}
	}
	return nil, false
}
