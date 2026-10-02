package detectors

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// classReason names the finding of a missing cluster-scoped class a pod
// template references. The API server rejects pods naming an absent
// PriorityClass ("no PriorityClass with name ... was found") or
// RuntimeClass, so the controller's pods are never created.
var classReason = map[inventory.Kind]string{
	kube.KindPriorityClass: reasons.PriorityClassMissing,
	kube.KindRuntimeClass:  reasons.RuntimeClassMissing,
}

// classReferences reports PriorityClasses and RuntimeClasses a workload's
// pod template references but that do not exist. A workload owned by a
// present controller (a Deployment's ReplicaSet, a CronJob's Job) is
// left to its owner, so one missing class is reported once. Kinds that
// are not fully watched are never concluded missing.
func classReferences(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil || ownedByPresent(ctx, e.ID) {
		return nil
	}
	var out []detection.Finding
	for _, kind := range []inventory.Kind{
		kube.KindPriorityClass, kube.KindRuntimeClass,
	} {
		if f, ok := missingClass(ctx, e, kind); ok {
			out = append(out, f)
		}
	}
	return out
}

func missingClass(
	ctx detection.Context, e inventory.Entity, kind inventory.Kind,
) (detection.Finding, bool) {
	if !ctx.Synced(kind) {
		return detection.Finding{}, false
	}
	var missing []string
	for _, id := range ctx.Model.Related(
		e.ID, inventory.References, inventory.Outgoing,
	) {
		if id.Kind == kind && !ctx.Model.Exists(id) {
			missing = append(missing, id.Name)
		}
	}
	if len(missing) == 0 {
		return detection.Finding{}, false
	}
	label := string(kind)
	evidence := make([]detection.Evidence, 0, len(missing))
	for _, name := range missing {
		evidence = append(evidence,
			detection.Evidence{Label: label, Value: name})
	}
	return detection.Finding{
		Reason: classReason[kind], Severity: detection.Critical,
		Summary: "Pod template references " + classTitle(kind) + " " +
			strings.Join(missing, ", ") + ", which does not exist; " +
			"its pods are rejected",
		Evidence: evidence,
	}, true
}

func classTitle(kind inventory.Kind) string {
	if kind == kube.KindRuntimeClass {
		return "RuntimeClass"
	}
	return "PriorityClass"
}

// ownedByPresent reports whether the entity has an owner in the model.
func ownedByPresent(ctx detection.Context, id inventory.EntityID) bool {
	for _, owner := range ctx.Model.Related(
		id, inventory.OwnedBy, inventory.Outgoing,
	) {
		if ctx.Model.Exists(owner) {
			return true
		}
	}
	return false
}
