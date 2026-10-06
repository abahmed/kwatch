package inventory

import (
	"testing"
	"time"
)

func TestStatsCountNotesRingsAndBaselines(t *testing.T) {
	m := NewModel(Options{})
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	id := EntityID{Kind: "pod", Namespace: "ns", Name: "a"}
	seen := Observation{Kind: Observed, Entity: id, At: at}
	if _, err := m.Apply(seen); err != nil {
		t.Fatal(err)
	}
	_, _ = m.Apply(Observation{Kind: Noted, Entity: id, At: at,
		Note: Note{Warning: true, Reason: "BackOff", Count: 1}})
	_, _ = m.Apply(Observation{Kind: Changed, Entity: id, At: at,
		Change: Change{Created: true}})
	m.Baselines().Add(id, MetricReadySeconds, 3, at)
	s := m.Stats()
	if s.Notes != 1 || s.Baselines != 1 || s.Churn != 1 || s.Changes != 1 {
		t.Fatalf("stats %+v", s)
	}
}
