package explain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// numberRevisions gives the fixture's ReplicaSets their revision numbers
// and creation times: api-1 is revision 13, api-2 is revision 14.
func numberRevisions(f *fixture) {
	for name, revision := range map[string]string{"api-1": "13",
		"api-2": "14"} {
		created := t0
		if name == "api-2" {
			created = t0.Add(time.Minute)
		}
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: inventory.CoreID(kube.KindReplicaSet, "shop", name),
			Attributes: map[string]inventory.Value{
				kube.AttrRevision: inventory.Text(revision),
				kube.AttrCreated:  inventory.Time(created)}})
	}
}

func editChange(
	deployment inventory.EntityID, minutes int, fields ...inventory.FieldChange,
) inventory.Change {
	return inventory.Change{Entity: deployment, Fields: fields,
		At: t0.Add(time.Duration(minutes) * time.Minute)}
}

// A new revision that fails alone names what it changed from the healthy
// one, likeliest culprit first, from the edits between the two
// ReplicaSets' creation.
func TestScoreRevisionNamesWhatTheNewRevisionChanged(t *testing.T) {
	f := rolloutFixture(t, false)
	numberRevisions(f)
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "api")
	f.changes[deployment] = []inventory.Change{
		editChange(deployment, -5, inventory.FieldChange{
			Path: "containers[app].env.OLD", Before: "a", After: "b"}),
		editChange(deployment, 1,
			inventory.FieldChange{Path: "containers[app].args",
				Before: "", After: "--x"},
			inventory.FieldChange{Path: "containers[app].resources." +
				"limits.memory", Before: "512Mi", After: "256Mi"},
			inventory.FieldChange{Path: "spec.replicas",
				Before: "2", After: "3"}),
	}

	got := scoreOf(t, f, deployment, scoreRevision)

	requireWeight(t, got, RevisionWeight)
	assert.Equal(t, rootcause.ProofNewRevisionFails, got.code)
	assert.Equal(t, 14, got.count)
	assert.Equal(t, []string{"containers[app].args",
		"containers[app].resources.limits.memory"}, pathsOf(got.edits),
		"the edit before the previous revision and replicas are left out")
	assert.Contains(t, got.text, "containers[app].args")
}

func TestScoreRevisionWithoutSeenEditsStillComparesRevisions(t *testing.T) {
	f := rolloutFixture(t, false)
	numberRevisions(f)
	deployment := inventory.CoreID(kube.KindDeployment, "shop", "api")

	got := scoreOf(t, f, deployment, scoreRevision)

	requireWeight(t, got, RevisionWeight)
	assert.Empty(t, got.edits)
}

func pathsOf(fields []inventory.FieldChange) []string {
	var out []string
	for _, f := range fields {
		out = append(out, f.Path)
	}
	return out
}
