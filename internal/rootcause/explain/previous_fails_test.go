package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// releaseFixture is web with ReplicaSets at revisions 1 (seen at t0)
// and 2 (seen at +5m, with the release), one pod under each, both
// crash-looping when oldFails, and a template change of the Deployment
// at +5m; the new pod crashes from +6m.
func releaseFixture(t *testing.T, oldFails bool) (*fixture, Cause) {
	f := newFixture(t)
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	f.add(dep)
	var pods []inventory.EntityID
	for i, revision := range []string{"1", "2"} {
		rs := inventory.CoreID(kube.KindReplicaSet, "shop", "web-"+revision)
		if _, err := f.model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: "test", Entity: rs,
			At: t0.Add(time.Duration(i*5) * time.Minute),
			Attributes: map[string]inventory.Value{
				kube.AttrRevision: inventory.Text(revision)}}); err != nil {
			t.Fatal(err)
		}
		f.relate(rs, inventory.OwnedBy, dep)
		pod := inventory.CoreID(kube.KindPod, "shop", "web-"+revision+"-x")
		f.add(pod, containerOf(pod))
		f.relate(pod, inventory.OwnedBy, rs)
		f.relate(containerOf(pod), inventory.PartOf, pod)
		pods = append(pods, pod)
	}
	f.change(dep, 5, "spec.template.spec.containers[app].image")
	f.fail(containerOf(pods[1]), detection.ModeCrashLoop, failingH, 6, "")
	if oldFails {
		f.fail(containerOf(pods[0]), detection.ModeCrashLoop, failingH,
			7, "")
	}
	return f, Cause{Root: dep, Row: "rollout",
		Covers:  []inventory.EntityID{containerOf(pods[1])},
		Changes: f.changes[dep]}
}

// A release whose previous version crashes the same way is not blamed,
// and no rollback is offered.
func TestRecordPreviousVersionFailsTooDropsTheRelease(t *testing.T) {
	f, c := releaseFixture(t, true)

	got := f.snapshot().Record(c)

	if got.Change != nil || got.RollbackRevision != "" ||
		!got.PreviousFails {
		t.Fatalf("record = change %v, revision %q, previous fails %v; "+
			"want no change, no revision, previous fails",
			got.Change, got.RollbackRevision, got.PreviousFails)
	}
}

// The twin: the previous version is healthy, so the release is blamed
// and the revision before it is the rollback target.
func TestRecordPreviousVersionHealthyKeepsTheRelease(t *testing.T) {
	f, c := releaseFixture(t, false)

	got := f.snapshot().Record(c)

	if got.Change == nil || got.RollbackRevision != "1" ||
		got.PreviousFails {
		t.Fatalf("record = change %v, revision %q, previous fails %v; "+
			"want the change and revision 1", got.Change,
			got.RollbackRevision, got.PreviousFails)
	}
}

// A workload held at zero replicas is not broken by a template change
// that left its replica count alone: own-change does not explain it.
// The twin: a change that set the replicas to zero does.
func TestOwnChangeExplainsZeroOnlyWhenItSetTheReplicas(t *testing.T) {
	for _, tt := range []struct {
		paths []string
		want  bool
	}{
		{[]string{"containers[app].image"}, false},
		{[]string{"containers[app].image", "spec.replicas"}, true},
	} {
		f := newFixture(t)
		dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
		f.add(dep)
		f.change(dep, 5, tt.paths...)
		f.fail(dep, detection.ModeScaledToZero, failingH, 6, "")

		match, _ := newView(f.snapshot()).selfMatch(dep, dep)

		if got := match.row.Name == "own-change"; got != tt.want {
			t.Errorf("change of %v: own-change = %v, want %v", tt.paths,
				got, tt.want)
		}
	}
}
