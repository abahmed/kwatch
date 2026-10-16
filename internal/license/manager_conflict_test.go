package license

import (
	"context"
	"testing"
	"time"
)

func TestManagerConflictRetriesFastThenHourly(t *testing.T) {
	h := newHarness(t)
	h.worker.bound, h.worker.cluster = true, "other-cluster"
	ctx := context.Background()

	if d := h.run(); d != 5*time.Minute {
		t.Fatalf("first conflict delay = %v", d)
	}
	if st := h.m.Status(); st.State != StateConflict {
		t.Fatalf("status = %+v", st)
	}
	since := h.m.conflictSince
	if since.IsZero() {
		t.Fatal("conflict start not recorded")
	}
	// Every five minutes for the first hour.
	for range 11 {
		h.clock.Advance(5 * time.Minute)
		if d := h.m.cycle(ctx); d != 5*time.Minute {
			t.Fatalf("delay at %v = %v", h.clock.Now().Sub(since), d)
		}
	}
	// The streak keeps its start: the state did not change, so no new log.
	if !h.m.conflictSince.Equal(since) {
		t.Fatal("conflict streak restarted")
	}
	h.clock.Advance(5 * time.Minute)
	if d := h.m.cycle(ctx); d != time.Hour {
		t.Fatalf("delay after the first hour = %v", d)
	}
	h.clock.Advance(time.Hour)
	if d := h.m.cycle(ctx); d != time.Hour {
		t.Fatalf("hourly delay = %v", d)
	}
	if got := h.worker.count("/license/activate"); got != 14 {
		t.Fatalf("activations = %d", got)
	}
}

func TestManagerConflictEndsWhenReleased(t *testing.T) {
	h := newHarness(t)
	h.worker.bound, h.worker.cluster = true, "other-cluster"
	h.run()
	h.worker.bound = false // the owner transferred the license
	h.clock.Advance(5 * time.Minute)
	d := h.m.cycle(context.Background())
	if st := h.m.Status(); st.State != StateActive {
		t.Fatalf("status = %+v", st)
	}
	if d != 24*time.Hour {
		t.Fatalf("delay after recovery = %v", d)
	}
	if !h.m.conflictSince.IsZero() {
		t.Fatal("conflict streak not cleared")
	}
}

func TestManagerConflictSurvivesNetworkBlip(t *testing.T) {
	h := newHarness(t)
	h.worker.bound, h.worker.cluster = true, "other-cluster"
	h.run()
	since := h.m.conflictSince
	h.worker.set(func(string) (int, map[string]any, map[string]string) {
		return 503, map[string]any{"error": "down"}, nil
	})
	h.clock.Advance(5 * time.Minute)
	h.m.cycle(context.Background())
	h.worker.set(nil)
	h.clock.Advance(5 * time.Minute)
	if d := h.m.cycle(context.Background()); d != 5*time.Minute {
		t.Fatalf("delay = %v", d)
	}
	if !h.m.conflictSince.Equal(since) {
		t.Fatal("a failed attempt restarted the streak")
	}
}
