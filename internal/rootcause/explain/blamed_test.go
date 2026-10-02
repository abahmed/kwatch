package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func rolloutOf(dep inventory.EntityID, at time.Time) inventory.Change {
	return inventory.Change{Entity: dep, At: at,
		Fields: []inventory.FieldChange{{Path: "containers[web].image",
			Before: "web:1", After: "web:2"}}}
}

// A Deployment degraded for days does not hide the rollout that just
// broke its pods: the old state predates every change, so the onset of
// the covered failures decides.
func TestBlamedChangesLongStandingDegradedRootBlamesRecentRollout(
	t *testing.T,
) {
	f := newFixture(t)
	pod := f.workload("shop", "web", 1)[0]
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	s := f.snapshot()
	s.Findings = map[inventory.EntityID][]detection.Finding{
		dep: {{Entity: dep, Mode: "Unavailable", Health: degradedH,
			Since: t0.Add(-48 * time.Hour)}},
		pod: {{Entity: pod, Mode: "CrashLoop", Health: failingH,
			Since: t0.Add(2 * time.Minute)}},
	}
	rollout := rolloutOf(dep, t0)
	c := Cause{Root: dep, Covers: []inventory.EntityID{pod},
		Changes: []inventory.Change{rollout}}

	got := s.BlamedChanges(c)
	if len(got) != 1 || !got[0].At.Equal(t0) {
		t.Fatalf("blamed = %+v, want the rollout", got)
	}
}

// Of the root's findings, only those after its last healthy mark count:
// an older finding belongs to an earlier episode.
func TestBlamedChangesRootOnsetStartsAfterLastHealthyMark(t *testing.T) {
	f := newFixture(t)
	node := inventory.CoreID(kube.KindNode, "", "n1")
	s := f.snapshot()
	f.model.MarkHealth(node, false, "", t0.Add(-time.Minute))
	s.Findings = map[inventory.EntityID][]detection.Finding{
		node: {
			{Entity: node, Mode: "Old", Health: degradedH,
				Since: t0.Add(-time.Hour)},
			{Entity: node, Mode: "MemoryPressure", Health: failingH,
				Since: t0.Add(3 * time.Minute)},
		},
	}
	edit := inventory.Change{Entity: node, At: t0.Add(time.Minute),
		Fields: []inventory.FieldChange{{Path: "spec.taints"}}}
	late := inventory.Change{Entity: node, At: t0.Add(5 * time.Minute),
		Fields: []inventory.FieldChange{{Path: "spec.taints"}}}
	c := Cause{Root: node, Changes: []inventory.Change{edit, late}}

	got := s.BlamedChanges(c)
	if len(got) != 1 || !got[0].At.Equal(edit.At) {
		t.Fatalf("blamed = %+v, want only the edit before the new "+
			"finding", got)
	}
}

// Without covered failures, a root that predates every change keeps its
// own onset, so nothing after it is blamed.
func TestBlamedChangesOldRootWithoutCoversBlamesNothing(t *testing.T) {
	f := newFixture(t)
	dep := inventory.CoreID(kube.KindDeployment, "shop", "web")
	s := f.snapshot()
	s.Findings = map[inventory.EntityID][]detection.Finding{
		dep: {{Entity: dep, Mode: "Unavailable", Health: degradedH,
			Since: t0.Add(-48 * time.Hour)}},
	}
	c := Cause{Root: dep, Changes: []inventory.Change{rolloutOf(dep, t0)}}
	if got := s.BlamedChanges(c); len(got) != 0 {
		t.Fatalf("blamed = %+v, want none", got)
	}
}
