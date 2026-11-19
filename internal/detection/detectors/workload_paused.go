package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultPausedEdit is how long a Deployment may stay paused with an
// edit that has not rolled out. A pause for a canary or a batch of edits
// lasts minutes; one that outlives that is usually forgotten.
const DefaultPausedEdit = 30 * time.Minute

// pausedReason is the Progressing condition reason of a paused Deployment.
const pausedReason = "DeploymentPaused"

// pausedEdit reports a paused Deployment whose pod template was edited
// after it was paused: the controller will not roll the edit out until it
// is resumed. A paused Deployment with nothing pending changes nothing,
// and is no finding.
func pausedEdit(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if e.ID.Kind != kube.KindDeployment || ctx.Model == nil {
		return detection.Finding{}, false
	}
	_, reason, since := condition(e, "Progressing")
	if reason != pausedReason || !editedSince(ctx, e.ID, since) {
		return detection.Finding{}, false
	}
	if !sustained(ctx, "paused-edit", since, DefaultPausedEdit) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.DeploymentPausedEdit, Severity: detection.Warning,
		Since: since,
		Summary: "Deployment is paused with a pod template edit that " +
			"has not rolled out, for " +
			format.Duration(ctx.Now.Sub(since)),
		Evidence: []detection.Evidence{{
			Label: "condition", Value: "Progressing " + pausedReason}},
	}, true
}

// pauseSlack is how far before the paused condition an edit still counts
// as made with the pause: one write can set spec.paused and the template.
const pauseSlack = time.Minute

// editedSince reports a change to the Deployment's pod template at or
// after the pause: any recorded change that is not the pause itself or a
// replica count.
func editedSince(
	ctx detection.Context, id inventory.EntityID, paused time.Time,
) bool {
	for _, change := range ctx.Model.Changes(id, paused.Add(-pauseSlack)) {
		for _, field := range change.Fields {
			if field.Path != "spec.paused" && field.Path != "spec.replicas" {
				return true
			}
		}
	}
	return false
}
