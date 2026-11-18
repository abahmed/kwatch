package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// revisionFixture is a workload of kind with ControllerRevisions at
// revisions 4 (newest) and 3, and one pod made from revision 3 that is
// ready or not, or none (pod ""). hash is the pod's
// controller-revision-hash label.
func revisionFixture(
	t *testing.T, kind inventory.Kind, hash, pod string,
) (*fixture, inventory.EntityID) {
	f := newFixture(t)
	w := inventory.CoreID(kind, "db", "pg")
	f.add(w)
	for name, revision := range map[string]string{
		"pg-new": "4", "pg-old": "3",
	} {
		cr := inventory.CoreID(kube.KindControllerRevision, "db", name)
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: cr, Attributes: map[string]inventory.Value{
				kube.AttrRevision: inventory.Text(revision)}})
		f.relate(cr, inventory.OwnedBy, w)
	}
	if pod != "" {
		id := inventory.CoreID(kube.KindPod, "db", "pg-0")
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: id, Attributes: map[string]inventory.Value{
				kube.AttrReady: inventory.Bool(pod == "ready"),
				kube.AttrLabels: inventory.Text(
					"app=pg," + kube.LabelRevisionHash + "=" + hash)}})
		f.relate(id, inventory.OwnedBy, w)
	}
	return f, w
}

// A StatefulSet or DaemonSet rolls back to the ControllerRevision before
// its newest, and only with a ready pod of that revision as evidence it
// served. A StatefulSet pod names the revision whole, a DaemonSet pod
// by the hash after the workload's name.
func TestControllerRevisionRollbackNeedsServedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, hash, pod string
		kind            inventory.Kind
		want            bool
	}{
		{"statefulset ready pod", "pg-old", "ready",
			kube.KindStatefulSet, true},
		{"daemonset ready pod", "old", "ready", kube.KindDaemonSet, true},
		{"never ready", "pg-old", "unready", kube.KindStatefulSet, false},
		{"no pod left", "", "", kube.KindStatefulSet, false},
		{"ready pod of the new revision", "pg-new", "ready",
			kube.KindStatefulSet, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, w := revisionFixture(t, tc.kind, tc.hash, tc.pod)
			s := f.snapshot()
			if got := rollbackRevision(s.Model, w); got != "3" {
				t.Fatalf("rollback revision = %q, want 3", got)
			}
			if got := s.previousServed(w); got != tc.want {
				t.Fatalf("previous served = %v, want %v", got, tc.want)
			}
		})
	}
}

// A covered pod finds its StatefulSet through its owner.
func TestControllerRevisionRollbackFromPod(t *testing.T) {
	f, _ := revisionFixture(t, kube.KindStatefulSet, "pg-old", "ready")
	s := f.snapshot()
	pod := inventory.CoreID(kube.KindPod, "db", "pg-0")
	if got := rollbackRevision(s.Model, pod); got != "3" {
		t.Fatalf("rollback revision = %q, want 3", got)
	}
	if !s.previousServed(pod) {
		t.Fatal("previous revision served, want true")
	}
}

// One revision is nothing to go back to.
func TestControllerRevisionRollbackNeedsTwoRevisions(t *testing.T) {
	f := newFixture(t)
	w := inventory.CoreID(kube.KindDaemonSet, "ops", "agent")
	f.add(w)
	cr := inventory.CoreID(kube.KindControllerRevision, "ops", "agent-a")
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: cr,
		Attributes: map[string]inventory.Value{
			kube.AttrRevision: inventory.Text("1")}})
	f.relate(cr, inventory.OwnedBy, w)
	s := f.snapshot()
	if got := rollbackRevision(s.Model, w); got != "" {
		t.Fatalf("rollback revision = %q, want none", got)
	}
}
