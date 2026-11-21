package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeClaimPinsVolume is a bound claim that keeps a deleted volume from
// going away: the volume protection finalizer waits for the claim.
const ModeClaimPinsVolume detection.Mode = "ClaimPinsVolume"

// pinnedVolumeRows blame the bound claim for a volume whose deletion
// waits.
var pinnedVolumeRows = []Row{{
	Name: RuleClaimPinsVolume,
	Cause: Side{Kind: kube.KindPVC,
		Modes: []detection.Mode{ModeClaimPinsVolume}},
	Link: LinkUses,
	Effect: Side{Kind: kube.KindPV,
		Modes: []detection.Mode{detection.ModeStuckDeleting}},
	Prior: 0.75,
}}

// boundClaims are the live claims bound to a volume whose deletion is
// stuck.
func (v *view) boundClaims(volume inventory.EntityID) []inventory.EntityID {
	if volume.Kind != kube.KindPV ||
		!v.hasMode(volume, []detection.Mode{detection.ModeStuckDeleting}) {
		return nil
	}
	var out []inventory.EntityID
	for _, claim := range v.s.Model.Related(
		volume, inventory.References, inventory.Incoming,
	) {
		e, ok := v.s.Model.Entity(claim)
		if claim.Kind == kube.KindPVC && ok &&
			attrText(e, kube.AttrPhase) == claimBound {
			out = append(out, claim)
		}
	}
	return out
}

// pinnedVolumeHops lead from a stuck volume to the claims bound to it.
func (v *view) pinnedVolumeHops(volume inventory.EntityID) []hop {
	return hopsTo(LinkUses, v.boundClaims(volume)...)
}

// claimPins reports whether claim is bound to the stuck volume effect.
func (v *view) claimPins(claim, effect inventory.EntityID) bool {
	return containsID(v.boundClaims(effect), claim) ||
		containsID(v.restoringClaims(effect), claim)
}
