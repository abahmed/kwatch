package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// SignalDiskFull is a write that failed because the filesystem is
// full: ENOSPC in its usual spellings, and a filesystem quota.
const SignalDiskFull Signal = "disk-full-error"

// claimBound is the phase of a claim bound to its volume.
const claimBound = "Bound"

// storageModes are the pseudo modes of the storage chain. A full disk
// is the claim's problem: the claim is the size a person sets and
// resizes, while the PersistentVolume is only the storage the claim
// asked for. So the claim a crashing pod mounts is full when the pod
// says so, and a volume a Bound claim names is never "missing": the
// binding proves it exists, even when kwatch does not list volumes.
// ok is false when the kind is not part of the storage chain.
func (v *view) storageModes(
	id, effect inventory.EntityID, link LinkType,
) (modes []modeHealth, ok bool) {
	switch id.Kind {
	case kube.KindPVC:
		// The usage detector reports VolumeFull when it can read
		// volume stats; the pseudo mode of the same name comes from
		// the errors of the pods that mount the claim, for clusters
		// where it cannot.
		if link == LinkMounts && v.claimFull(id, effect) {
			return []modeHealth{{mode: detection.ModeVolumeFull,
				health: detection.Failing, pseudo: true}}, true
		}
	case kube.KindPV:
		if v.boundByClaim(id) {
			return nil, true
		}
	}
	return nil, false
}

// claimNearFullPct is the used share from which a claim may be the
// full disk an error reports; the usage detector warns at the same
// level.
const claimNearFullPct = 85.0

// claimFull reports whether the effect's full-disk error is about the
// claim. Usage stats decide when kwatch has them. Without them the
// error must name the claim or its volume, unless the claim is the
// only one the pod mounts.
func (v *view) claimFull(claim, effect inventory.EntityID) bool {
	text := v.text(effect)
	if !hasSignal(SignalDiskFull, text) {
		return false
	}
	if e, ok := v.s.Model.Entity(claim); ok {
		if used, ok := attrNumber(e, kube.AttrVolumeUsedPct); ok {
			return used >= claimNearFullPct
		}
	}
	if v.namesClaim(claim, strings.ToLower(text)) {
		return true
	}
	pod, ok := v.unitOf(effect)
	return ok && len(v.s.Model.Related(pod, inventory.Mounts,
		inventory.Outgoing)) == 1
}

// namesClaim reports whether lower-case text names the claim or the
// volume bound to it as a whole word or path segment.
func (v *view) namesClaim(claim inventory.EntityID, text string) bool {
	names := []string{claim.Name}
	for _, volume := range v.s.Model.Related(
		claim, inventory.References, inventory.Outgoing,
	) {
		names = append(names, volume.Name)
	}
	for _, name := range names {
		if containsWord(text, strings.ToLower(name)) {
			return true
		}
	}
	return false
}

// containsWord reports whether word occurs in text with no
// [a-z0-9-] byte on either side.
func containsWord(text, word string) bool {
	for from := 0; word != "" && from <= len(text)-len(word); {
		i := strings.Index(text[from:], word)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(word)
		if (start == 0 || !wordByte(text[start-1])) &&
			(end == len(text) || !wordByte(text[end])) {
			return true
		}
		from = start + 1
	}
	return false
}

func wordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-'
}

// boundByClaim reports whether a Bound claim names the volume.
func (v *view) boundByClaim(volume inventory.EntityID) bool {
	for _, claim := range v.s.Model.Related(
		volume, inventory.References, inventory.Incoming,
	) {
		if claim.Kind != kube.KindPVC {
			continue
		}
		if e, ok := v.s.Model.Entity(claim); ok &&
			attrText(e, kube.AttrPhase) == claimBound {
			return true
		}
	}
	return false
}
