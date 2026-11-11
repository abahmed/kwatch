package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A workload a GitOps controller deploys is undone in Git, a Helm
// release with Helm; anything else keeps the rollout undo to a revision.
func TestRolloutStepsUndoWhereTheWorkloadIsDeployed(t *testing.T) {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	last := func(app string) string {
		steps := rolloutSteps(api, "7", nil, app, nil)
		s := steps[len(steps)-1]
		return s.Text + " | " + s.Command
	}
	assert.Equal(t, "Revert the change in Git (argocd/shop): a rollback "+
		"in the cluster would be overwritten by the next sync | ",
		last("argocd/shop"))
	assert.Contains(t, last("flux/apps"), "Revert the change in Git")
	assert.Equal(t, "Roll back the Helm release api (changes the "+
		"cluster) | helm rollback api -n shop", last("helm/api"))
	assert.Contains(t, last(""), "--to-revision=")
}
