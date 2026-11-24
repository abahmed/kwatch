package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// revisionAt records when the ReplicaSet was created and since when it
// is the current revision.
func revisionAt(f *fixture, rs inventory.EntityID, at time.Time) {
	f.t.Helper()
	_, err := f.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Entity: rs, Source: "test", At: at,
		Attributes: map[string]inventory.Value{
			kube.AttrCreated:  inventory.Time(at),
			kube.AttrRevision: inventory.Text("7"),
		}})
	if err != nil {
		f.t.Fatal(err)
	}
}

// A workload change is blamed for its crashing pods only when it rolled
// out what they run: an edit of the replica count or the pause starts
// no revision, and pods of a ReplicaSet weeks old (a cold start on new
// nodes) do not run anything the change made.
func TestRolloutBlamedOnlyForTheRevisionItMade(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		created time.Duration
		blamed  bool
	}{
		{"new revision", "spec.template.spec.containers[0].image",
			time.Minute, true},
		{"pause only", "spec.paused", time.Minute, false},
		{"revision weeks old", "spec.template.spec.containers[0].image",
			-21 * 24 * time.Hour, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			pods := f.workload("shop", "api", 2)
			deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
			revisionAt(f, inventory.CoreID(kube.KindReplicaSet, "shop",
				"api-1"), t0.Add(tc.created))
			f.change(deploy, 1, tc.path)
			for _, pod := range pods {
				f.fail(containerOf(pod), "CrashLoop", failingH, 2, "")
			}
			c, ok := f.explain().CauseOf(containerOf(pods[0]))
			blamed := ok && c.Root == deploy && c.Row == RuleRollout
			if blamed != tc.blamed {
				t.Fatalf("rollout blamed = %v, want %v (cause %+v)",
					blamed, tc.blamed, c)
			}
		})
	}
}
