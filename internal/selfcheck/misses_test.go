package selfcheck

import (
	"fmt"
	"testing"
	"time"
)

func missAt(id string, at time.Time) Miss {
	return Miss{ID: id, Rule: "node", Root: "node//n1", Opened: at,
		Resolved: at}
}

func TestMissesKeepNewestFirstWithinTheWindow(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	var s Misses
	s.Record(missAt("old", now.AddDate(0, 0, -8)))
	s.Record(missAt("a", now.Add(-2*time.Hour)))
	s.Record(missAt("b", now.Add(-time.Hour)))
	s.Record(missAt("a", now)) // the same incident again replaces it
	got := s.List(now)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("misses = %+v", got)
	}
}

func TestMissesAreBounded(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	var s Misses
	for i := range MaxMisses + 10 {
		s.Record(missAt(fmt.Sprint(i), now.Add(time.Duration(i)*time.Second)))
	}
	got := s.List(now.Add(time.Hour))
	if len(got) != MaxMisses || got[0].ID != fmt.Sprint(MaxMisses+9) {
		t.Fatalf("kept %d, newest %q", len(got), got[0].ID)
	}
}

func TestMissesRestoreSkipsDamagedEntries(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	var s Misses
	s.Restore([]Miss{{ID: ""}, {ID: "x"}, missAt("ok", now)})
	if got := s.List(now); len(got) != 1 || got[0].ID != "ok" {
		t.Fatalf("restored %+v", got)
	}
}
