package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// servedFixture is web with ReplicaSets at revisions 7 (newest) and 5,
// revision 5 at ready replicas, and one pod of revision 5 that is
// ready or not, or none (pod "").
func servedFixture(
	t *testing.T, ready float64, pod string,
) (*fixture, inventory.EntityID) {
	f := newFixture(t)
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.add(dep)
	for name, revision := range map[string]string{
		"web-7": "7", "web-5": "5",
	} {
		rs := inventory.CoreID(kube.KindReplicaSet, "shop", name)
		attrs := map[string]inventory.Value{
			kube.AttrRevision: inventory.Text(revision)}
		if name == "web-5" {
			attrs[kube.AttrReadyReplicas] = inventory.Number(ready)
		}
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: rs, Attributes: attrs})
		f.relate(rs, inventory.OwnedBy, dep)
	}
	if pod != "" {
		id := inventory.CoreID(kube.KindPod, "shop", "web-5-x")
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: id, Attributes: map[string]inventory.Value{
				kube.AttrReady: inventory.Bool(pod == "ready")}})
		f.relate(id, inventory.OwnedBy,
			inventory.CoreID(kube.KindReplicaSet, "shop", "web-5"))
	}
	return f, dep
}

// A rollback target must have served: ready replicas of its ReplicaSet
// or a ready pod of it. Existing, at zero ready replicas with no pod or
// only an unready one, is no record of it running.
func TestPreviousServedNeedsPositiveEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ready float64
		pod   string
		want  bool
	}{
		{"ready replicas", 2, "", true},
		{"ready pod", 0, "ready", true},
		{"zero replicas, no pod", 0, "", false},
		{"never ready", 0, "unready", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, dep := servedFixture(t, tc.ready, tc.pod)
			s := f.snapshot()
			if got := rollbackRevision(s.Model, dep); got != "5" {
				t.Fatalf("rollback revision = %q, want 5", got)
			}
			if got := s.previousServed(dep); got != tc.want {
				t.Fatalf("previous served = %v, want %v", got, tc.want)
			}
		})
	}
}

// Without revision numbers, the ReplicaSet kwatch saw appear last
// before the newest is the one judged; a Deployment with one
// ReplicaSet has none to go back to.
func TestPreviousServedWithoutRevisionNumbers(t *testing.T) {
	f := newFixture(t)
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.add(dep)
	for i, name := range []string{"web-a", "web-b"} {
		rs := inventory.CoreID(kube.KindReplicaSet, "shop", name)
		if _, err := f.model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "test", Entity: rs,
			At: t0.Add(time.Duration(i) * time.Minute),
			Attributes: map[string]inventory.Value{
				kube.AttrReadyReplicas: inventory.Number(
					float64(1 - i))}}); err != nil {
			t.Fatal(err)
		}
		f.relate(rs, inventory.OwnedBy, dep)
	}
	if !f.snapshot().previousServed(dep) {
		t.Fatal("earlier ReplicaSet with a ready replica: want served")
	}
	lone := newFixture(t)
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "web-1")
	lone.add(dep, rs)
	lone.relate(rs, inventory.OwnedBy, dep)
	if lone.snapshot().previousServed(dep) {
		t.Fatal("one ReplicaSet: nothing to roll back to")
	}
}

// The record carries the evidence: a blamed release whose previous
// revision never became ready keeps its revision but is not marked
// served, so no undo is offered from it.
func TestRecordMarksWhetherThePreviousRevisionServed(t *testing.T) {
	for _, ready := range []float64{0, 3} {
		f, dep := servedFixture(t, ready, "")
		got := f.snapshot().Record(Cause{Root: dep,
			Covers:  []inventory.EntityID{dep},
			Changes: []inventory.Change{{Entity: dep, At: t0}}})
		if got.RollbackRevision != "5" || got.PreviousServed != (ready > 0) {
			t.Fatalf("ready %v: record revision %q, served %v", ready,
				got.RollbackRevision, got.PreviousServed)
		}
	}
}

// A Helm rollback goes to the release's last deployed revision, read
// from the release kwatch sees; none for another tool or an unseen
// release.
func TestHelmGoodRevisionReadsTheRelease(t *testing.T) {
	f := newFixture(t)
	for _, ns := range []string{"shop", "ops"} {
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: kube.HelmReleaseID(ns, "billing"),
			Attributes: map[string]inventory.Value{
				kube.AttrHelmGoodRevision: inventory.Number(4)}})
	}
	dep := inventory.CoreID(kube.KindDeployment, "shop", "billing")
	model := f.snapshot().Model
	for app, want := range map[string]string{
		"helm/billing": "4", "helm/ops/billing": "4",
		"argocd/billing": "", "helm/other": "", "": "",
	} {
		got := helmGoodRevision(model,
			inventory.Change{Entity: dep, App: app})
		if got != want {
			t.Errorf("%q: revision %q, want %q", app, got, want)
		}
	}
}
