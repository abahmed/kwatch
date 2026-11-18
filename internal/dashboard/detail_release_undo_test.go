package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/release"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// servedCause blames a change of deployment name in ns, scored score,
// whose rollback returns to revision and has served.
func servedCause(
	name, ns, revision string, score float64,
) *rootcause.CauseRecord {
	return &rootcause.CauseRecord{Score: score,
		Change: &inventory.Change{Entity: inventory.EntityID{
			Kind: "Deployment", Namespace: ns, Name: name}},
		RollbackRevision: revision, PreviousServed: true}
}

func apiRelease(app string) release.Record {
	return release.Record{Kind: "deployment", Namespace: "shop",
		Name: "api", App: app}
}

func TestUndoNeedsABlamingCauseAtLikely(t *testing.T) {
	r := apiRelease("")
	none, _ := undoFor(r, nil)
	assert.Equal(t, release.Undo{}, none)
	other, _ := undoFor(r, servedCause("web", "shop", "3", 0.9))
	assert.Equal(t, release.Undo{}, other, "another workload's change")
	possible, _ := undoFor(r, servedCause("api", "shop", "3", 0.3))
	assert.Equal(t, release.Undo{}, possible)
	rival := servedCause("api", "shop", "3", 0.9)
	rival.Rival = servedCause("api", "shop", "2", 0.88)
	near, _ := undoFor(r, rival)
	assert.Equal(t, release.Undo{}, near, "a near rival is only possible")
}

func TestUndoWithoutServedEvidenceSaysWhy(t *testing.T) {
	c := servedCause("api", "shop", "3", 0.6)
	c.PreviousServed = false
	u, rev := undoFor(apiRelease(""), c)
	assert.Empty(t, u.Command)
	assert.False(t, u.Mutating)
	assert.Equal(t, "kwatch has no record of revision 3 running "+
		"healthily, so it does not suggest rolling back to it", u.Text)
	assert.Empty(t, rev)
	// Managed workloads need the evidence too.
	u, _ = undoFor(apiRelease("argocd/shop"), c)
	assert.Contains(t, u.Text, "no record of revision 3")
	// Nothing known to roll back to: nothing offered.
	c.RollbackRevision = ""
	u, _ = undoFor(apiRelease(""), c)
	assert.Equal(t, release.Undo{}, u)
}

func TestUndoWithEvidenceFollowsTheDeployMethod(t *testing.T) {
	c := servedCause("api", "shop", "3", 0.6)
	u, rev := undoFor(apiRelease(""), c)
	assert.Equal(t, "kubectl rollout undo deployment/api -n shop "+
		"--to-revision=3", u.Command)
	assert.True(t, u.Mutating)
	assert.Equal(t, "3", rev)

	u, _ = undoFor(apiRelease("flux/apps"), c)
	assert.Empty(t, u.Command)
	assert.Contains(t, u.Text, "Revert the change in Git")

	c.HelmRollbackRevision = "5"
	u, rev = undoFor(apiRelease("helm/ops/api"), c)
	assert.Equal(t, "helm rollback api 5 -n ops", u.Command)
	assert.Equal(t, "5", rev)

	c.HelmRollbackRevision = ""
	u, _ = undoFor(apiRelease("helm/api"), c)
	assert.Empty(t, u.Command)
	assert.Contains(t, u.Text, "no record of a deployed revision of "+
		"Helm release api")

	c.RollbackRevision = "latest"
	u, _ = undoFor(apiRelease(""), c)
	assert.Empty(t, u.Command, "never a bare undo without a revision")
}

// A cause covering api and web that blames web's release offers web's
// revision on web's release only, as the message offers a rollback
// only of the blamed workload: never on api's, whose name sorts first.
func TestUndoOnlyOnTheBlamedWorkloadsRelease(t *testing.T) {
	c := servedCause("web", "shop", "5", 0.9)
	api, _ := undoFor(apiRelease(""), c)
	assert.Equal(t, release.Undo{}, api)
	web := release.Record{Kind: "deployment", Namespace: "shop",
		Name: "web"}
	u, rev := undoFor(web, c)
	assert.Equal(t, "kubectl rollout undo deployment/web -n shop "+
		"--to-revision=5", u.Command)
	assert.Equal(t, "5", rev)
}
