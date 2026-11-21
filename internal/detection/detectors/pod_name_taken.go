package detectors

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// conflictingPod reads the pod a controller's ConflictingController
// event names: "pod data/db-1 is not controlled by StatefulSet db".
var conflictingPod = regexp.MustCompile(
	`pod ([a-z0-9.-]+)/([a-z0-9.-]+) is not controlled by`)

// nameTakenFindings reports a pod nobody owns that holds the name a
// controller of its namespace needs for its own pod: the controller
// says so in a ConflictingController event. The pod is the cause to
// act on, so the controller's own event is left out (conflictNamesPod).
// It is a digest line: the other replicas keep serving.
func nameTakenFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil || len(ctx.Model.Related(e.ID, inventory.OwnedBy,
		inventory.Outgoing)) > 0 {
		return nil
	}
	for _, kind := range []inventory.Kind{kube.KindStatefulSet,
		kube.KindDaemonSet} {
		for _, owner := range ctx.Model.EntitiesIn(kind, e.ID.Namespace) {
			for _, note := range ctx.Model.Notes(owner,
				ctx.Now.Add(-EventWindow)) {
				if !note.Warning || note.Reason != "ConflictingController" ||
					!namesPod(note, e.ID) {
					continue
				}
				ctx.RecheckAfter(note.At.Add(EventWindow).Sub(ctx.Now))
				return []detection.Finding{{
					Reason: reasons.PodNameTaken, Severity: detection.Info,
					Since: note.At,
					Summary: "Pod holds the name " + string(kind) + " " +
						owner.Name + " needs for its own pod",
					Evidence: []detection.Evidence{
						{Label: "event", Value: note.Message}},
				}}
			}
		}
	}
	return nil
}

func namesPod(note inventory.Note, pod inventory.EntityID) bool {
	m := conflictingPod.FindStringSubmatch(note.Message)
	return m != nil && m[1] == pod.Namespace && m[2] == pod.Name
}

// conflictNamesPod reports a ConflictingController event whose pod the
// model has and nobody owns: the pod carries the finding instead.
func conflictNamesPod(
	ctx detection.Context, e inventory.Entity, note inventory.Note,
) bool {
	if ctx.Model == nil || note.Reason != "ConflictingController" {
		return false
	}
	m := conflictingPod.FindStringSubmatch(note.Message)
	if m == nil {
		return false
	}
	pod := inventory.CoreID(kube.KindPod, m[1], m[2])
	return ctx.Model.Exists(pod) && len(ctx.Model.Related(pod,
		inventory.OwnedBy, inventory.Outgoing)) == 0
}

// nameHeldByPod reports a controller that is short of replicas because
// a pod nobody owns holds a name it needs: the pod's own finding says
// so, and the shortfall adds nothing to it.
func nameHeldByPod(ctx detection.Context, e inventory.Entity) bool {
	if ctx.Model == nil {
		return false
	}
	for _, note := range ctx.Model.Notes(e.ID, ctx.Now.Add(-EventWindow)) {
		if note.Warning && conflictNamesPod(ctx, e, note) {
			return true
		}
	}
	return false
}

// ConflictingPodOf is the pod a ConflictingController event on a
// controller names.
func ConflictingPodOf(note inventory.Note) (inventory.EntityID, bool) {
	if note.Reason != "ConflictingController" {
		return inventory.EntityID{}, false
	}
	m := conflictingPod.FindStringSubmatch(note.Message)
	if m == nil {
		return inventory.EntityID{}, false
	}
	return inventory.CoreID(kube.KindPod, m[1], m[2]), true
}
