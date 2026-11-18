package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/release"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// kindCause blames a change of the workload kind/name in shop at
// likely, whose rollback returns to revision.
func kindCause(
	kind inventory.Kind, name, revision string, served bool,
) *rootcause.CauseRecord {
	return &rootcause.CauseRecord{Score: 0.6,
		Change: &inventory.Change{Entity: inventory.CoreID(kind, "shop",
			name)},
		RollbackRevision: revision, PreviousServed: served}
}

// StatefulSets and DaemonSets follow the Deployment's rules on the
// Releases page: a known revision with served evidence rolls back to
// it, without the evidence the page says why, and a GitOps-managed one
// is changed in Git.
func TestUndoRulesHoldForStatefulSetsAndDaemonSets(t *testing.T) {
	sts := release.Record{Kind: "statefulset", Namespace: "shop",
		Name: "db"}
	u, rev := undoFor(sts, kindCause("statefulset", "db", "4", true))
	assert.Equal(t, "kubectl rollout undo statefulset/db -n shop "+
		"--to-revision=4", u.Command)
	assert.True(t, u.Mutating)
	assert.Equal(t, "4", rev)

	u, rev = undoFor(sts, kindCause("statefulset", "db", "4", false))
	assert.Empty(t, u.Command)
	assert.Equal(t, "kwatch has no record of revision 4 running "+
		"healthily, so it does not suggest rolling back to it", u.Text)
	assert.Empty(t, rev)

	ds := release.Record{Kind: "daemonset", Namespace: "shop",
		Name: "agent", App: "argocd/ops"}
	u, _ = undoFor(ds, kindCause("daemonset", "agent", "2", true))
	assert.Empty(t, u.Command)
	assert.Contains(t, u.Text, "Revert the change in Git (argocd/ops)")
}
