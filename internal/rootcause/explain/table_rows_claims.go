package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// kindResourceClaim is the entity kind of a ResourceClaim (dynamic
// resource allocation).
var kindResourceClaim = kube.KindFor("ResourceClaim")

// claimRows blame a device claim nothing could satisfy for the pod that
// waits for it.
var claimRows = []Row{{
	Name: "resource-claim-unallocated",
	Cause: Side{Group: AnyGroup, Kind: kindResourceClaim,
		Modes: []detection.Mode{detection.ModeDeviceUnallocated}},
	Link:   LinkUses,
	Effect: podSide(detection.ModePending, detection.ModeUnschedulable),
	Prior:  0.8,
}}

// claimHops lead from a pod that has not started to the ResourceClaims
// it owns. The claim speaks for itself: it is a cause only while it has
// its own unallocated finding.
func (v *view) claimHops(pod inventory.EntityID) []hop {
	if !v.hasMode(pod, []detection.Mode{
		detection.ModePending, detection.ModeUnschedulable}) {
		return nil
	}
	var out []hop
	for _, claim := range v.s.Model.Related(
		pod, inventory.OwnedBy, inventory.Incoming,
	) {
		if claim.Kind == kindResourceClaim {
			out = append(out, hop{link: LinkUses, to: claim})
		}
	}
	return out
}
