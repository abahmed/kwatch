package inventory

import (
	"testing"
	"time"
)

func TestRestoreChangesKeepsTimesAndActors(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("deployment", "shop", "api")
	at := time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC)
	got := m.RestoreChanges([]Change{{
		Entity: id, At: at, Actor: "argocd", Revision: "7",
		Fields: []FieldChange{{Path: "spec.template", After: "b"}},
	}, {Entity: id}})

	if got != 1 {
		t.Fatalf("kept %d, want 1 (the change without a time is dropped)", got)
	}
	changes := m.Changes(id, time.Time{})
	if len(changes) != 1 || changes[0].Actor != "argocd" ||
		!changes[0].At.Equal(at) {
		t.Fatalf("changes = %+v", changes)
	}
	if sets := m.RecentChangeSets(at); len(sets) != 1 {
		t.Fatalf("the change must feed change sets, got %d", len(sets))
	}
}

func TestPruneKeepingKeepsChangesLongerThanNotes(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("deployment", "shop", "api")
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	old := now.Add(-5 * time.Hour)
	m.RestoreChanges([]Change{{Entity: id, At: old, Actor: "bob"}})
	m.RestoreHealth(id, HealthMark{At: old, Revision: "3"})

	m.PruneKeeping(now.Add(-2*time.Hour), now.Add(-ChangeRetention))

	if len(m.Changes(id, time.Time{})) != 1 ||
		len(m.HealthMarks(id, time.Time{})) != 1 {
		t.Fatal("a 5h old change and mark must survive a 24h keep")
	}
	m.PruneKeeping(now, now)
	if len(m.Changes(id, time.Time{})) != 0 {
		t.Fatal("Prune must still drop old changes")
	}
}
