package inventory

import (
	"testing"
	"time"
)

func TestModelCountMatchesEntities(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewModel(Options{})
	for _, id := range []EntityID{
		{Kind: "Pod", Namespace: "a", Name: "p1"},
		{Kind: "Pod", Namespace: "b", Name: "p2"},
		{Kind: "Node", Name: "n1"},
	} {
		_, _ = m.Apply(Observation{Kind: Observed, Source: "t", At: at,
			Entity: id})
	}
	_, _ = m.Apply(Observation{Kind: Gone, Source: "t", At: at,
		Entity: EntityID{Kind: "Pod", Namespace: "b", Name: "p2"}})

	for _, kind := range []Kind{"Pod", "Node", "Service"} {
		if got, want := m.Count(kind), len(m.Entities(kind)); got != want {
			t.Fatalf("Count(%s) = %d, want %d", kind, got, want)
		}
	}
}
