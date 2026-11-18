package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A workload a GitOps controller deploys is undone in Git, a Helm
// release with Helm; anything else keeps the rollout undo to a revision.
func TestRolloutStepsUndoWhereTheWorkloadIsDeployed(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	last := func(app string) string {
		cause := &rootcause.CauseRecord{Root: api, Score: 0.9,
			RollbackRevision: "7", PreviousServed: true,
			HelmRollbackRevision: "6",
			Change:               &inventory.Change{Entity: api, App: app}}
		steps := rolloutSteps(api, cause, nil, nil)
		s := steps[len(steps)-1]
		return s.Text + " | " + s.Command
	}
	assert.Equal(t, "Revert the change in Git (argocd/shop): a rollback "+
		"in the cluster would be overwritten by the next sync | ",
		last("argocd/shop"))
	assert.Contains(t, last("flux/apps"), "Revert the change in Git")
	assert.Equal(t, "Roll back the Helm release api to revision 6, its "+
		"last deployed one (changes the cluster) | helm rollback api 6 "+
		"-n shop", last("helm/api"))
	assert.Contains(t, last("helm/ops/api"), "helm rollback api 6 -n ops")
	assert.Contains(t, last(""), "--to-revision=")
}

// A workload that fails from its creation has no earlier version to go
// back to: no revert or rollback is offered, its error's steps are.
func TestCreatedWorkloadIsNotOfferedAnUndo(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	box := inventory.EntityID{Kind: kube.KindContainer, Namespace: "shop",
		Name: "api-7d-a/app"}
	p := incident.Incident{Root: api, Cause: &rootcause.CauseRecord{
		Root: api, Rule: "rollout",
		Change: &inventory.Change{Entity: api, Created: true,
			App: "argocd/shop"}}}
	members := []detection.Finding{{Entity: box,
		Reason: reasons.ImagePullBackOff, Severity: detection.Critical,
		Mode: detection.ModeImagePull}}

	steps := nextSteps(p, members)

	assert.NotEmpty(t, steps)
	for _, s := range steps {
		assert.NotContains(t, s.Text, "Revert")
		assert.NotContains(t, s.Text, "Roll back")
		assert.False(t, s.Mutating, s.Text)
	}
}

// A Helm rollback goes to the release's last deployed revision; without
// one known, no rollback is offered and the step says why.
func TestHelmUndoNeedsTheLastDeployedRevision(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	step := helmUndo(api, "helm/api", "")
	assert.Empty(t, step.Command)
	assert.False(t, step.Mutating)
	assert.Equal(t, "kwatch has no record of a deployed revision of Helm "+
		"release api to go back to, so it does not suggest rolling it "+
		"back", step.Text)
}
