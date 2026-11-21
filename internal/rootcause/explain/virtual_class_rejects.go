package explain

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ModeClassRejects is a StorageClass whose secret reference or
// parameters make the provisioner fail every claim of the class.
const ModeClassRejects detection.Mode = "ClassRejects"

// classRejectPattern is a provisioning error about the class itself: a
// provisioner Secret that is gone or a parameter that is refused.
var classRejectPattern = regexp.MustCompile(
	`(?i)failed to get secret|secrets? "[^"]*" not found|` +
		`invalid parameter|unknown parameter|reserved prefix`)

// classRows blame the class when the claim's provisioning error is
// about the class's own configuration.
var classRows = []Row{
	{
		Name: RuleStorageClassRejects,
		Cause: Side{Kind: kube.KindStorageClass,
			Modes: []detection.Mode{ModeClassRejects}},
		Link:   LinkUses,
		Effect: unboundClaimSide(),
		Prior:  0.8,
	},
	{
		// The same class, seen from the pod that waits for the claim.
		Name: RuleStorageClassRejectsPending,
		Cause: Side{Kind: kube.KindStorageClass,
			Modes: []detection.Mode{ModeClassRejects}},
		Link: LinkUses,
		Effect: podSide(detection.ModePending, detection.ModeUnschedulable,
			detection.ModeCreating),
		Prior: 0.8,
	},
}

// rejectedClasses are the classes of the claims, among the effect (a
// claim) or the claims a pod mounts, whose error is about the class.
func (v *view) rejectedClasses(
	effect inventory.EntityID,
) []inventory.EntityID {
	claims := []inventory.EntityID{effect}
	if effect.Kind == kube.KindPod {
		claims = v.s.Model.Related(effect, inventory.Mounts,
			inventory.Outgoing)
	}
	var out []inventory.EntityID
	for _, claim := range claims {
		if claim.Kind != kube.KindPVC ||
			!classRejectPattern.MatchString(v.text(claim)) {
			continue
		}
		for _, id := range v.s.Model.Related(claim, inventory.References,
			inventory.Outgoing) {
			if id.Kind == kube.KindStorageClass {
				out = append(out, id)
			}
		}
	}
	return out
}

// classRejectHops lead from a claim, or a pod that mounts one, to the
// class its provisioning error is about.
func (v *view) classRejectHops(id inventory.EntityID) []hop {
	return hopsTo(LinkUses, v.rejectedClasses(id)...)
}

// classRejects reports whether the effect's claim error is about class.
func (v *view) classRejects(class, effect inventory.EntityID) bool {
	for _, id := range v.rejectedClasses(effect) {
		if id == class {
			return true
		}
	}
	return false
}
