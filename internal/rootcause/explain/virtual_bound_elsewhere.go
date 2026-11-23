package explain

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// boundElsewherePattern is the volume controller's answer to a claim it
// cannot bind to the volume that names it: group 1 is the volume.
var boundElsewherePattern = regexp.MustCompile(
	`volume "([^"]+)" already bound to a different claim`)

// boundElsewhereVolume is the existing volume the claim's own error says
// is already bound to a different claim.
func (v *view) boundElsewhereVolume(
	claim inventory.EntityID,
) (inventory.EntityID, bool) {
	m := boundElsewherePattern.FindStringSubmatch(v.text(claim))
	if m == nil {
		return inventory.EntityID{}, false
	}
	for _, id := range v.s.Model.CoreEntitiesNamed(kube.KindPV, m[1]) {
		return id, true
	}
	return inventory.EntityID{}, false
}

// refusedClaims are the claims, among the effect (a claim) or the claims
// a pod mounts, whose error names a volume that is already bound to a
// different claim, with that volume.
func (v *view) refusedClaims(
	effect inventory.EntityID,
) map[inventory.EntityID]inventory.EntityID {
	claims := []inventory.EntityID{effect}
	if effect.Kind == kube.KindPod {
		claims = v.s.Model.Related(effect, inventory.Mounts,
			inventory.Outgoing)
	}
	out := map[inventory.EntityID]inventory.EntityID{}
	for _, claim := range claims {
		if claim.Kind != kube.KindPVC {
			continue
		}
		if volume, ok := v.boundElsewhereVolume(claim); ok {
			out[claim] = volume
		}
	}
	return out
}

// boundElsewhereHops lead from a claim, or a pod that mounts one, to the
// volume the claim's error names.
func (v *view) boundElsewhereHops(id inventory.EntityID) []hop {
	var out []hop
	for _, volume := range v.refusedClaims(id) {
		out = append(out, hop{link: LinkUses, to: volume})
	}
	return out
}

// refusedAsBound reports whether the effect's claim error names the
// volume as already bound to a different claim.
func (v *view) refusedAsBound(volume, effect inventory.EntityID) bool {
	for _, named := range v.refusedClaims(effect) {
		if named == volume {
			return true
		}
	}
	return false
}
