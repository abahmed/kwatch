package license

import (
	"context"
	"testing"
	"time"
)

// A wall clock that stepped forward once and was then corrected must not
// keep Pro locked until real time catches up: the next lease the server
// signs carries its own time and pulls the guard back.
func TestManagerClockGuardRecoversFromAForwardStep(t *testing.T) {
	h := newHarness(t)
	h.run()
	h.clock.Advance(20 * 24 * time.Hour) // a bogus step forward
	if st := h.m.Status(); st.State.ProEnabled() {
		t.Fatalf("the stepped clock still reads Pro: %+v", st)
	}
	// NTP corrects it, an hour after the first lease was issued.
	h.clock.Advance(-20*24*time.Hour + time.Hour)
	h.m.cycle(context.Background())
	if st := h.m.Status(); !st.State.ProEnabled() {
		t.Fatalf("Pro did not recover after the correction: %+v", st)
	}
	if got := h.m.clock.MaxSeen; got.After(h.clock.Now().Add(time.Hour)) {
		t.Fatalf("the guard stayed ahead of the server: %v", got)
	}
}

// Only a lease newer than the cached one may pull the guard back, so a
// replay of an old response cannot undo a rolled-back clock check.
func TestManagerClockGuardIgnoresAReplayedLease(t *testing.T) {
	h := newHarness(t)
	h.run()
	old := h.leases.lease
	h.clock.Advance(8 * 24 * time.Hour) // lease and grace are over
	if st := h.m.Status(); st.State != StateExpired {
		t.Fatalf("expired: %+v", st)
	}
	h.clock.Advance(-7 * 24 * time.Hour) // clock set back into the lease
	h.worker.set(func(p string) (int, map[string]any, map[string]string) {
		if p == "/license/renew" || p == "/license/activate" {
			return 200, map[string]any{"lease": old}, nil
		}
		return 0, nil, nil
	})
	h.m.cycle(context.Background())
	if st := h.m.Status(); st.State.ProEnabled() {
		t.Fatalf("a replayed lease revived Pro: %+v", st)
	}
}
