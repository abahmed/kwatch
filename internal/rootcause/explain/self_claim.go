package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// claimConfigModes is ModeNoStorageClass for a Pending claim that names
// no class while nothing can give it a volume: the cluster has no
// default StorageClass and no unclaimed volume without a class that is
// large enough. A claim with a selector wants a volume by its labels,
// which is a different shortage. It needs both kinds listed: a kind
// kwatch cannot see may hold the default.
func (v *view) claimConfigModes(claim inventory.EntityID) []detection.Mode {
	if claim.Kind != kube.KindPVC || !v.s.verifiable(kube.KindStorageClass) ||
		!v.s.verifiable(kube.KindPV) {
		return nil
	}
	e, ok := v.s.Model.Entity(claim)
	if !ok || attrText(e, kube.AttrPhase) != "Pending" ||
		!attrBool(e, kube.AttrClassUnset) ||
		attrBool(e, kube.AttrHasSelector) || v.boundVolumeNamed(claim) ||
		v.defaultClassExists() {
		return nil
	}
	need, _ := attrNumber(e, kube.AttrRequested)
	if v.staticVolumeFits(need) {
		return nil
	}
	return []detection.Mode{ModeNoStorageClass}
}

// boundVolumeNamed reports a claim that names a volume to bind to.
func (v *view) boundVolumeNamed(claim inventory.EntityID) bool {
	for _, id := range v.s.Model.Related(
		claim, inventory.References, inventory.Outgoing,
	) {
		if id.Kind == kube.KindPV {
			return true
		}
	}
	return false
}

// defaultClassExists reports a StorageClass marked as the default.
func (v *view) defaultClassExists() bool {
	for _, id := range v.s.Model.Entities(kube.KindStorageClass) {
		if e, ok := v.s.Model.Entity(id); ok && attrBool(e,
			kube.AttrDefaultClass) {
			return true
		}
	}
	return false
}

// staticVolumeFits reports an Available volume of no class with at
// least need bytes: a claim of no class may bind to it.
func (v *view) staticVolumeFits(need float64) bool {
	for _, id := range v.s.Model.Entities(kube.KindPV) {
		e, ok := v.s.Model.Entity(id)
		if !ok || attrText(e, kube.AttrPhase) != "Available" {
			continue
		}
		size, _ := attrNumber(e, kube.AttrCapacity)
		if size >= need && !v.hasClass(id) {
			return true
		}
	}
	return false
}

// hasClass reports a volume that names a StorageClass.
func (v *view) hasClass(volume inventory.EntityID) bool {
	for _, id := range v.s.Model.Related(
		volume, inventory.References, inventory.Outgoing,
	) {
		if id.Kind == kube.KindStorageClass {
			return true
		}
	}
	return false
}
