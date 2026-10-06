package incident

import (
	"testing"
	"time"
)

// restoreAfterGap restores an announced incident into a fresh manager a
// long monitoring gap later, none of its findings returning, and follows
// the manager's own wake-ups. It returns when the resolve was decided.
func restoreAfterGap(
	t *testing.T, mutate func(*Record), gap, grace time.Duration,
) time.Time {
	t.Helper()
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))
	records := old.m.Export()
	if mutate != nil {
		mutate(&records[0])
	}
	start := at(gap)
	fresh := newRig(t, Config{})
	fresh.m.Restore(records, start.Add(grace))
	now := start
	for range 20 {
		ds, wake := fresh.m.Tick(now)
		for _, d := range ds {
			if d.Action == Resolve {
				return now
			}
		}
		if wake == 0 {
			t.Fatalf("no wake-up pending at %v, incident never resolves",
				now.Sub(start))
		}
		now = now.Add(wake)
	}
	t.Fatal("incident did not resolve")
	return now
}

// A workload that was already healthy at startup resolves a hold after
// the restore grace, however long kwatch was not watching.
func TestRestoredOpenIncidentResolvesPromptlyAfterGap(t *testing.T) {
	grace := 10 * time.Minute
	got := restoreAfterGap(t, nil, 8*time.Hour, grace)
	if want := at(8*time.Hour + grace + DefaultHold); !got.Equal(want) {
		t.Fatalf("resolved at %v, want %v", got, want)
	}
}

// A record saved while recovering keeps its old RecoveringSince; the
// hold is long over, so it resolves as soon as the grace is.
func TestRestoredRecoveringIncidentResolvesAtGraceEnd(t *testing.T) {
	grace := 10 * time.Minute
	got := restoreAfterGap(t, func(r *Record) {
		r.State = Recovering
		r.RecoveringSince = at(2 * time.Minute)
	}, 8*time.Hour, grace)
	if want := at(8*time.Hour + grace); !got.Equal(want) {
		t.Fatalf("resolved at %v, want %v", got, want)
	}
}
