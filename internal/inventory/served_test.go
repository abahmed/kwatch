package inventory

import (
	"fmt"
	"testing"
	"time"
)

var servedBase = time.Unix(1_800_000_000, 0).UTC()

func servedNames(served []ServedRevision) []string {
	var out []string
	for _, s := range served {
		out = append(out, s.Name)
	}
	return out
}

// A workload remembers at most MaxServedRevisions revisions, the most
// recently served; the oldest sighting goes first.
func TestMarkServedKeepsTheMostRecentRevisions(t *testing.T) {
	m := NewModel(Options{})
	w := CoreID("deployment", "shop", "web")
	for i := range MaxServedRevisions + 2 {
		at := servedBase.Add(time.Duration(i) * 2 * time.Hour)
		if !m.MarkServed(w, fmt.Sprintf("web-%d", i), at) {
			t.Fatalf("revision %d: want a change", i)
		}
	}

	got := servedNames(m.ServedRevisions(w, time.Time{}))

	want := []string{"web-2", "web-3", "web-4", "web-5", "web-6"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("served = %v, want %v", got, want)
	}
}

// Seeing the same revision again changes nothing until its sighting is
// an hour old; then it moves to the newest place.
func TestMarkServedRefreshesHourly(t *testing.T) {
	m := NewModel(Options{})
	w := CoreID("statefulset", "db", "pg")
	m.MarkServed(w, "pg-a", servedBase)
	m.MarkServed(w, "pg-b", servedBase.Add(time.Minute))

	if m.MarkServed(w, "pg-a", servedBase.Add(30*time.Minute)) {
		t.Fatal("a sighting within the hour: want no change")
	}
	if !m.MarkServed(w, "pg-a", servedBase.Add(2*time.Hour)) {
		t.Fatal("a sighting after the hour: want a change")
	}
	got := servedNames(m.ServedRevisions(w, time.Time{}))
	if fmt.Sprint(got) != "[pg-b pg-a]" {
		t.Fatalf("served = %v, want pg-b then pg-a", got)
	}
}

// A revision last seen serving more than ServedRetention ago is
// forgotten, by a later sighting and by the prune.
func TestServedRevisionsExpire(t *testing.T) {
	m := NewModel(Options{})
	w := CoreID("deployment", "shop", "web")
	m.MarkServed(w, "web-old", servedBase)
	later := servedBase.Add(ServedRetention + time.Hour)
	m.MarkServed(w, "web-new", later)
	if got := servedNames(m.ServedRevisions(w, time.Time{})); fmt.Sprint(
		got) != "[web-new]" {
		t.Fatalf("served = %v, want only web-new", got)
	}

	gone := CoreID("daemonset", "ops", "agent")
	m.RestoreServed(gone, []ServedRevision{{Name: "agent-a",
		At: servedBase}}, time.Time{})
	m.Prune(servedBase.Add(time.Hour))
	if len(m.ServedRevisions(gone, time.Time{})) != 1 {
		t.Fatal("prune within the retention: want the revision kept")
	}
	m.PruneKeeping(later, later.Add(-ChangeRetention))
	if len(m.ServedRevisions(gone, time.Time{})) != 0 {
		t.Fatal("prune past the retention: want the revision gone")
	}
}

// Deleting a workload forgets what it served, whether kwatch saw the
// deletion or reconstructed it after a restart.
func TestServedRevisionsGoWithTheWorkload(t *testing.T) {
	m := NewModel(Options{})
	w := CoreID("deployment", "shop", "web")
	if _, err := m.Apply(Observation{Kind: Observed, Source: "test",
		Entity: w, At: servedBase}); err != nil {
		t.Fatal(err)
	}
	m.MarkServed(w, "web-1", servedBase)
	if _, err := m.Apply(Observation{Kind: Gone, Source: "test",
		Entity: w, At: servedBase.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if got := m.ServedRevisions(w, time.Time{}); len(got) != 0 {
		t.Fatalf("after deletion served = %v, want none", got)
	}

	m.MarkServed(w, "web-2", servedBase.Add(time.Hour))
	m.ForgetServed(w)
	if got := m.ServedRevisions(w, time.Time{}); len(got) != 0 {
		t.Fatalf("after forget served = %v, want none", got)
	}
}

// A restore keeps the bound and leaves out what is past since and what
// is already remembered.
func TestRestoreServedIsBounded(t *testing.T) {
	m := NewModel(Options{})
	w := CoreID("deployment", "shop", "web")
	var saved []ServedRevision
	for i := range 8 {
		saved = append(saved, ServedRevision{Name: fmt.Sprintf("web-%d", i),
			At: servedBase.Add(time.Duration(i) * time.Hour)})
	}
	saved = append(saved, ServedRevision{Name: "web-7",
		At: servedBase.Add(time.Hour)}, ServedRevision{})

	m.RestoreServed(w, saved, servedBase.Add(2*time.Hour))

	got := servedNames(m.ServedRevisions(w, time.Time{}))
	want := []string{"web-3", "web-4", "web-5", "web-6", "web-7"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("served = %v, want %v", got, want)
	}
}
