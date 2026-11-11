package release

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestUndoFollowsWhatDeploysTheWorkload(t *testing.T) {
	for name, tc := range map[string]struct {
		app, revision string
		want          Undo
	}{
		"argo cd": {app: "argocd/shop", want: Undo{Text: "Revert the " +
			"change in Git (argocd/shop): a rollback in the cluster " +
			"would be overwritten by the next sync"}},
		"flux": {app: "flux/apps", want: Undo{Text: "Revert the change " +
			"in Git (flux/apps): a rollback in the cluster would be " +
			"overwritten by the next sync"}},
		"helm": {app: "helm/api", want: Undo{Text: "Roll back the Helm " +
			"release api (changes the cluster)",
			Command: "helm rollback api -n shop", Mutating: true}},
		"helm release in another namespace": {app: "helm/ops/api",
			want: Undo{Text: "Roll back the Helm release api " +
				"(changes the cluster)",
				Command: "helm rollback api -n ops", Mutating: true}},
		"helm chart without a release": {app: "helm-chart/api-1.2.0",
			want: Undo{Text: "Roll back (changes the cluster)",
				Command:  "kubectl rollout undo statefulset/db -n shop",
				Mutating: true}},
		"by hand": {want: Undo{Text: "Roll back (changes the cluster)",
			Command:  "kubectl rollout undo statefulset/db -n shop",
			Mutating: true}},
		"known revision": {revision: "7", want: Undo{
			Text:     "Roll back to revision 7 (changes the cluster)",
			Command:  "kubectl rollout undo statefulset/db -n shop --to-revision=7",
			Mutating: true}},
	} {
		got := UndoFor("statefulset", "shop", "db", tc.app, tc.revision)
		if name == "helm" {
			got = UndoFor("deployment", "shop", "api", tc.app, "")
		}
		assert.Equal(t, tc.want, got, name)
	}
}

func TestBookKeepsTheNewestWithinItsBounds(t *testing.T) {
	var b Book
	for i := range MaxRecords + 20 {
		b.add(Record{ID: fmt.Sprint(i), At: t0.Add(time.Duration(i) *
			time.Minute)})
	}
	assert.Len(t, b.Records, MaxRecords)
	assert.Equal(t, fmt.Sprint(MaxRecords+19), b.Records[0].ID)
	assert.True(t, b.prune(t0.Add(KeepFor+30*time.Minute)))
	for _, r := range b.Records {
		assert.False(t, r.At.Before(t0.Add(30*time.Minute)), r.ID)
	}
}

func TestDetailQuotesAStartErrorOfANewPod(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(1, 0, 1, deadlineReason)
	w.change(t0, "", image("api:1", "api:2"))
	pod := inventory.CoreID(kube.KindPod, "shop", "api-2-x")
	container := kube.ContainerID("shop", "api-2-x", "app")
	at := t0.Add(time.Minute)
	w.m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: at, Entity: pod})
	w.m.Apply(inventory.Observation{Kind: inventory.Related,
		Source: "test", At: at, Entity: pod, Relation: inventory.OwnedBy,
		Targets: []inventory.EntityID{w.id}})
	w.m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: at, Entity: container,
		Attributes: map[string]inventory.Value{
			kube.AttrRestarts: inventory.Number(4),
			kube.AttrLastErrorLine: inventory.Text(
				"panic: setting X must be set"),
			kube.AttrLastStarted:  inventory.Time(at),
			kube.AttrLastFinished: inventory.Time(at.Add(5 * time.Second)),
		}})
	w.m.Apply(inventory.Observation{Kind: inventory.Related,
		Source: "test", At: at, Entity: container,
		Relation: inventory.OwnedBy, Targets: []inventory.EntityID{pod}})
	var b Book
	Observe(&b, View{Model: w.m, Detail: true}, t0.Add(2*time.Minute))
	d := b.Records[0].Detail
	if assert.NotNil(t, d) {
		assert.Equal(t, "panic: setting X must be set", d.ErrorLine)
		if assert.NotNil(t, d.RestartsAfter) {
			assert.InDelta(t, 120, *d.RestartsAfter, 0.1)
		}
	}
	assert.Equal(t, Down, b.Records[0].Verdict)
}

// A full book does not take back a release older than all it keeps: an
// idle look changes nothing, so nothing is saved again.
func TestFullBookIgnoresReleasesOlderThanItKeeps(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(1, 1, 1, "")
	w.change(t0, "", image("api:1", "api:2"))
	var b Book
	for i := range MaxRecords {
		b.Records = append(b.Records, Record{
			ID:   fmt.Sprintf("deployment/shop/other-%d@1", i),
			At:   t0.Add(time.Duration(MaxRecords-i) * time.Minute),
			Kind: "deployment", Namespace: "shop",
			Name: fmt.Sprintf("other-%d", i), Verdict: Healthy, Final: true,
		})
	}
	now := t0.Add(24 * time.Hour)
	assert.False(t, Observe(&b, View{Model: w.m}, now))
	assert.Len(t, b.Records, MaxRecords)
	assert.Nil(t, b.find(recordID(w.id, t0, 1)))
}

// A pod the model saw a few seconds before the release was stamped is
// still the release's: its crash-loop restarts count.
func TestDetailCountsAPodSeenJustBeforeTheReleaseStamp(t *testing.T) {
	w := newWorld(kube.KindDeployment, "api")
	w.status(1, 0, 1, deadlineReason)
	w.change(t0, "", image("api:1", "api:2"))
	pod := inventory.CoreID(kube.KindPod, "shop", "api-2-x")
	container := kube.ContainerID("shop", "api-2-x", "app")
	at := t0.Add(-5 * time.Second)
	w.m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: at, Entity: pod})
	w.m.Apply(inventory.Observation{Kind: inventory.Related,
		Source: "test", At: at, Entity: pod, Relation: inventory.OwnedBy,
		Targets: []inventory.EntityID{w.id}})
	w.m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: at, Entity: container,
		Attributes: map[string]inventory.Value{
			kube.AttrRestarts: inventory.Number(30)}})
	w.m.Apply(inventory.Observation{Kind: inventory.Related,
		Source: "test", At: at, Entity: container,
		Relation: inventory.OwnedBy, Targets: []inventory.EntityID{pod}})
	var b Book
	Observe(&b, View{Model: w.m, Detail: true}, t0.Add(30*time.Minute))
	d := b.Records[0].Detail
	if assert.NotNil(t, d) && assert.NotNil(t, d.RestartsAfter) {
		assert.InDelta(t, 60, *d.RestartsAfter, 0.1)
	}
}
