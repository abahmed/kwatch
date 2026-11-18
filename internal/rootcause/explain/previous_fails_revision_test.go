package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// revisionReleaseFixture is a StatefulSet or DaemonSet pg with
// ControllerRevisions 3 (pg-old, seen at t0) and 4 (pg-new, seen at
// +5m with the release), pod pg-0 of the old revision and pg-1 of the
// new one, both crash-looping when oldFails, and a template change of
// the workload at +5m; the new pod crashes from +6m. A StatefulSet pod
// names its revision whole, a DaemonSet pod by the hash after "pg-".
func revisionReleaseFixture(
	t *testing.T, kind inventory.Kind, oldFails bool,
) (*fixture, Cause) {
	f := newFixture(t)
	w := inventory.CoreID(kind, "db", "pg")
	f.add(w)
	var pods []inventory.EntityID
	for i, rev := range []string{"old", "new"} {
		cr := inventory.CoreID(kube.KindControllerRevision, "db", "pg-"+rev)
		if _, err := f.model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "test", Entity: cr,
			At: t0.Add(time.Duration(i*5) * time.Minute),
			Attributes: map[string]inventory.Value{
				kube.AttrRevision: inventory.Text([]string{"3", "4"}[i])},
		}); err != nil {
			t.Fatal(err)
		}
		f.relate(cr, inventory.OwnedBy, w)
		hash := rev
		if kind == kube.KindStatefulSet {
			hash = "pg-" + rev
		}
		pod := inventory.CoreID(kube.KindPod, "db", []string{"pg-0",
			"pg-1"}[i])
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: pod, Attributes: map[string]inventory.Value{
				kube.AttrLabels: inventory.Text(
					"app=pg," + kube.LabelRevisionHash + "=" + hash)}})
		f.add(containerOf(pod))
		f.relate(pod, inventory.OwnedBy, w)
		f.relate(containerOf(pod), inventory.PartOf, pod)
		pods = append(pods, pod)
	}
	f.change(w, 5, "spec.template.spec.containers[app].image")
	f.fail(containerOf(pods[1]), detection.ModeCrashLoop, failingH, 6, "")
	if oldFails {
		f.fail(containerOf(pods[0]), detection.ModeCrashLoop, failingH,
			7, "")
	}
	return f, Cause{Root: w, Row: "rollout",
		Covers:  []inventory.EntityID{containerOf(pods[1])},
		Changes: f.changes[w]}
}

// A StatefulSet or DaemonSet release whose previous revision's pod
// crashes the same way is not blamed, and no rollback is offered; when
// the previous revision is healthy the release keeps the blame and the
// revision before it is the rollback target.
func TestRecordPreviousRevisionFailsTooForControllerRevisions(
	t *testing.T,
) {
	for _, kind := range []inventory.Kind{kube.KindStatefulSet,
		kube.KindDaemonSet} {
		for _, oldFails := range []bool{true, false} {
			f, c := revisionReleaseFixture(t, kind, oldFails)

			got := f.snapshot().Record(c)

			blamed := got.Change != nil && got.RollbackRevision == "3"
			dropped := got.Change == nil && got.RollbackRevision == "" &&
				!got.PreviousServed
			if got.PreviousFails != oldFails ||
				(oldFails && !dropped) || (!oldFails && !blamed) {
				t.Errorf("%s, previous fails %v: record = change %v, "+
					"revision %q, previous fails %v", kind, oldFails,
					got.Change, got.RollbackRevision, got.PreviousFails)
			}
		}
	}
}

// A revision kwatch first saw after the change (as a later fix
// attempt is) says nothing about the version before it.
func TestPreviousRevisionFailsIgnoresRevisionsSeenAfter(t *testing.T) {
	f, c := revisionReleaseFixture(t, kube.KindStatefulSet, true)
	c.Changes[0].At = t0.Add(-time.Minute)
	got := f.snapshot().previousFails(c.Changes[0], c.Covers)
	if got {
		t.Fatal("previous revision first seen after the change: " +
			"want not counted")
	}
}
