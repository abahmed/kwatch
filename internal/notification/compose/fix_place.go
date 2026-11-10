package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// weightPlace ranks the fix line inside its own part; nothing else
// shares the part.
const weightPlace = 0.5

// placeSentences name the exact place to change, as an action: "Fix:
// raise the memory limit of container app in deployment/api above
// 256Mi." (see fix_wording.go).
// The place comes from structured fields of the cause (or of the
// failure when no cause was found), never from message text. They say
// nothing when the cause is not a setting, or when two causes are
// still possible: a place for one of them would pick between them.
func placeSentences(f caseFacts) []sentence {
	place, ok := f.fixPlace()
	if !ok {
		return nil
	}
	said := false
	for _, s := range leadSentences(f) {
		said = said || valueNamed(s.text, place.Value)
	}
	return []sentence{{part: partPlace, weight: weightPlace,
		text: fixText(place, said)}}
}

// fixPlace finds the setting the incident comes from. The cause's own
// place wins; a failure's place counts only when no cause was found.
func (f caseFacts) fixPlace() (detection.Place, bool) {
	if f.p.Cause != nil {
		if f.p.Cause.Rival != nil || f.p.Cause.Place == nil {
			return detection.Place{}, false
		}
		return f.withObject(*f.p.Cause.Place)
	}
	for _, m := range failing(f.members) {
		if m.Place != nil && m.Entity.Namespace == f.p.Root.Namespace {
			return f.withObject(*m.Place)
		}
	}
	return detection.Place{}, false
}

// withObject fills in the object of a place that left it to the
// incident: the root, when the root is a workload.
func (f caseFacts) withObject(p detection.Place) (detection.Place, bool) {
	if !p.Valid() {
		return p, false
	}
	if p.Object == (inventory.EntityID{}) {
		if !isWorkloadKind(f.p.Root.Kind) {
			return p, false
		}
		p.Object = f.p.Root
	}
	return p, true
}

// isWorkloadKind reports a kind whose pods run containers.
func isWorkloadKind(kind inventory.Kind) bool {
	for _, k := range kube.WorkloadKinds {
		if k == kind {
			return true
		}
	}
	return false
}
