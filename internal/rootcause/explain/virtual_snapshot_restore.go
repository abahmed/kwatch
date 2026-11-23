package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// RuleClaimRestoresFromSnapshot names the row that blames a claim
// restored from a snapshot whose deletion waits.
const RuleClaimRestoresFromSnapshot = "claim-restores-from-snapshot"

// snapshotRestoreRows blame the claim that is restored from a
// snapshot for the snapshot's deletion that waits.
var snapshotRestoreRows = []Row{{
	Name: RuleClaimRestoresFromSnapshot,
	Cause: Side{Kind: kube.KindPVC,
		Modes: []detection.Mode{ModeClaimPinsVolume}},
	Link: LinkUses,
	Effect: Side{Group: snapshotGroup, Kind: kindVolumeSnapshot,
		Modes: []detection.Mode{detection.ModeStuckDeleting}},
	Prior: 0.75,
}}

// restoringClaims are the claims of the snapshot's namespace that are
// restored from a snapshot whose deletion is stuck.
func (v *view) restoringClaims(snap inventory.EntityID) []inventory.EntityID {
	if snap.Group != snapshotGroup || snap.Kind != kindVolumeSnapshot ||
		!v.hasMode(snap, []detection.Mode{detection.ModeStuckDeleting}) {
		return nil
	}
	var out []inventory.EntityID
	for _, claim := range v.s.Model.Entities(kube.KindPVC) {
		e, ok := v.s.Model.Entity(claim)
		if ok && claim.Namespace == snap.Namespace &&
			attrText(e, kube.AttrRestoreSnapshot) == snap.Name {
			out = append(out, claim)
		}
	}
	return out
}

// restoringHops lead from a snapshot whose deletion is stuck to the
// claims restored from it.
func (v *view) restoringHops(snap inventory.EntityID) []hop {
	return hopsTo(LinkUses, v.restoringClaims(snap)...)
}
