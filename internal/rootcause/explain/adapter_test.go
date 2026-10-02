package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestNewSnapshotWiresEveryReader(t *testing.T) {
	f := newFixture(t)
	pod := f.workload("shop", "api", 1)[0]
	active := map[inventory.EntityID][]detection.Finding{pod: {
		{Entity: pod, Mode: "CrashLoop", Health: failingH},
		{Entity: pod, Mode: "NotReady", Health: degradedH},
	}}
	s := NewSnapshot(f.model, active, nil, f.now)
	if len(s.Findings[pod]) != 2 || s.Links == nil || s.Changes == nil ||
		s.Outcomes == nil || s.Baseline == nil {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestModelChangesStopsAtUntil(t *testing.T) {
	f := newFixture(t)
	secret := inventory.CoreID(kube.KindSecret, "shop", "db-creds")
	f.add(secret)
	for _, minutes := range []int{1, 20} {
		f.apply(inventory.Observation{Kind: inventory.Changed,
			Entity: secret, Change: inventory.Change{
				At: t0.Add(time.Duration(minutes) * time.Minute)}})
	}
	got := ModelChanges{Reader: f.model}.Changes(secret, t0,
		t0.Add(10*time.Minute))
	if len(got) != 1 {
		t.Fatalf("changes = %v, want the first only", got)
	}
}

func TestKubeLinksResolvesPresentReferences(t *testing.T) {
	f := newFixture(t)
	pod := f.workload("shop", "api", 1)[0]
	secret := inventory.CoreID(kube.KindSecret, "shop", "db-creds")
	f.add(secret)
	f.relate(pod, inventory.References, secret)
	links := KubeLinks{Reader: f.model}.Links(pod)
	if len(links) != 1 || links[0].To != secret {
		t.Fatalf("links = %+v", links)
	}
}

func TestRecordKeepsWhatMessagesNeed(t *testing.T) {
	f := newFixture(t)
	effect := usesCase(f, kube.KindSecret, true)
	s := f.snapshot()
	cause, ok := Explain(s).CauseOf(effect)
	if !ok {
		t.Fatal("no cause for the secret case")
	}
	got := s.Record(cause)
	if got.Root.Name != "db-creds" || got.Change == nil ||
		len(got.Proof) == 0 || got.Score != cause.Confidence ||
		got.Rule != cause.Row {
		t.Fatalf("record = %+v", got)
	}
}

func TestRecordCarriesRootFindingsAndRollbackRevision(t *testing.T) {
	f := newFixture(t)
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	newRS := inventory.CoreID(kube.KindReplicaSet, "shop", "web-7")
	oldRS := inventory.CoreID(kube.KindReplicaSet, "shop", "web-5")
	pod := inventory.CoreID(kube.KindPod, "shop", "web-7-a")
	f.add(dep, pod)
	for rs, revision := range map[inventory.EntityID]string{
		newRS: "7", oldRS: "5",
	} {
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: rs, Attributes: map[string]inventory.Value{
				kube.AttrRevision: inventory.Text(revision)}})
		f.relate(rs, inventory.OwnedBy, dep)
	}
	f.relate(pod, inventory.OwnedBy, newRS)
	f.fail(dep, "ProgressDeadlineExceeded", failingH, 3, "")
	f.fail(dep, "Info", detection.Healthy, 3, "")
	got := f.snapshot().Record(Cause{
		Root: dep, Covers: []inventory.EntityID{pod},
		Changes: []inventory.Change{{Entity: dep, At: t0}},
	})
	if got.RollbackRevision != "5" {
		t.Fatalf("rollback revision = %q, want 5", got.RollbackRevision)
	}
	if len(got.RootFindings) != 1 {
		t.Fatalf("root findings = %+v, want the unhealthy one",
			got.RootFindings)
	}
}

func TestBlamedChangesDropScalesAndChangesAfterTheFailure(t *testing.T) {
	f := newFixture(t)
	pod := f.workload("shop", "web", 1)[0]
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	onset := t0.Add(time.Minute)
	s := f.snapshot()
	s.Findings = map[inventory.EntityID][]detection.Finding{pod: {{
		Entity: pod, Mode: "CrashLoop", Health: failingH, Since: onset,
	}}}
	rollout := inventory.Change{Entity: dep, At: t0,
		Fields: []inventory.FieldChange{{Path: "containers[web].image",
			Before: "web:1", After: "web:2"}}}
	scaleUp := inventory.Change{Entity: dep, At: t0.Add(30 * time.Second),
		Fields: []inventory.FieldChange{{Path: "replicas",
			Before: "2", After: "6"}}}
	later := rollout
	later.At = onset.Add(time.Minute)
	c := Cause{Root: dep, Covers: []inventory.EntityID{pod},
		Changes: []inventory.Change{rollout, scaleUp, later}}

	got := s.BlamedChanges(c)
	if len(got) != 1 || !got[0].At.Equal(t0) {
		t.Fatalf("blamed = %+v, want only the rollout", got)
	}
	if rec := s.Record(c); rec.Change == nil || !rec.Change.At.Equal(t0) {
		t.Fatalf("record change = %+v, want the rollout", rec.Change)
	}
	// A scale before the failure is kept when nothing else changed.
	c.Changes = []inventory.Change{scaleUp}
	if got := s.BlamedChanges(c); len(got) != 1 {
		t.Fatalf("blamed = %+v, want the scale", got)
	}
}
