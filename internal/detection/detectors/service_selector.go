package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
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
