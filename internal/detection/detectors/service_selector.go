package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultSelectorChangeWindow is how long after a selector or port edit
// a Service that selects nothing is still that edit's failure. Later it
// may be on purpose, as an empty Service usually is.
const DefaultSelectorChangeWindow = time.Hour

// selectsNothing reports a Service left without endpoints by its own
// selector or port edit. An empty Service is normally scaled to zero on
// purpose; one that emptied right after its selector changed was broken
// by that edit, while its pods may be perfectly healthy.
func selectsNothing(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	edit, ok := selectorEdit(ctx, e.ID)
	if !ok || !sustained(ctx, "selects-nothing", edit.At, DefaultNoEndpoints) {
		return detection.Finding{}, false
	}
	ctx.RecheckAfter(edit.At.Add(DefaultSelectorChangeWindow).
		Sub(ctx.Now))
	return detection.Finding{
		Reason: reasons.ServiceNoEndpoints, Severity: detection.Critical,
		Since: edit.At,
		Summary: "Service selects no pods since its " + edit.Fields[0].Path +
			" changed",
		Evidence: []detection.Evidence{{Label: "change",
			Value: edit.Fields[0].Before + " -> " + edit.Fields[0].After}},
	}, true
}

// selectorEdit returns the latest selector or port edit of a Service
// inside DefaultSelectorChangeWindow.
func selectorEdit(
	ctx detection.Context, id inventory.EntityID,
) (inventory.Change, bool) {
	changes := ctx.Model.Changes(id, ctx.Now.Add(
		-DefaultSelectorChangeWindow))
	for i := len(changes) - 1; i >= 0; i-- {
		for _, field := range changes[i].Fields {
			if field.Path == "spec.selector" || field.Path == "spec.ports" {
				edit := changes[i]
				edit.Fields = []inventory.FieldChange{field}
				return edit, true
			}
		}
	}
	return inventory.Change{}, false
}

// selectorMatchesNothing reports an exposed Service whose selector no
// pod and no workload pod template in its namespace carries. An empty
// Service is normally a workload scaled to zero, which still has a
// template carrying the labels; here nothing ever will serve the
// traffic an Ingress, route, LoadBalancer or NodePort sends it. Every
// source it reads must be synced: a missing one hides the pods.
func selectorMatchesNothing(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	selector := text(e, kube.AttrSelector)
	if ctx.Model == nil || selector == "<none>" || !labelsSynced(ctx) ||
		len(kube.SelectedPods(ctx.Model, e)) > 0 ||
		kube.SelectedByTemplate(ctx.Model, e) ||
		len(serviceExposures(ctx, e.ID)) == 0 ||
		!sustained(ctx, "selector-matches-nothing", e.FirstSeen,
			DefaultNoEndpoints) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.ServiceNoEndpoints, Severity: detection.Critical,
		Since: e.FirstSeen,
		Summary: "Service " + e.ID.Name + " selects " + selector +
			"; no pod or workload template in " + e.ID.Namespace +
			" has those labels",
		Evidence: []detection.Evidence{{Label: "selector",
			Value: selector}},
	}, true
}

// labelsSynced reports whether every kind that can carry pod labels is
// watched. It asks about each kind, so all the missing ones are noted.
func labelsSynced(ctx detection.Context) bool {
	synced := ctx.Synced(kube.KindPod)
	for _, kind := range kube.TemplateKinds() {
		synced = ctx.Synced(kind) && synced
	}
	return synced
}

// allSelectedPending reports a Service whose selected pods all exist
// but are all Pending: they have no address, so the EndpointSlice is
// empty although the Service is not scaled to zero.
func allSelectedPending(
	ctx detection.Context, e inventory.Entity,
) bool {
	if ctx.Model == nil || !ctx.Synced(kube.KindPod) {
		return false
	}
	pods := kube.SelectedPods(ctx.Model, e)
	for _, id := range pods {
		pod, ok := ctx.Model.Entity(id)
		if !ok || text(pod, kube.AttrPhase) != "Pending" {
			return false
		}
	}
	return len(pods) > 0
}
