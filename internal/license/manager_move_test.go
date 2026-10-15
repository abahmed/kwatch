package license

import (
	"context"
	"testing"
	"time"
)

// moveRig drives CheckMove with an in-memory request marker.
type moveRig struct {
	h    *harness
	m    *Manager
	at   time.Time
	outs []MoveOutcome
}

func newMoveRig(t *testing.T) *moveRig {
	r := &moveRig{h: newHarness(t)}
	r.m = r.h.newManager(r.h.key)
	r.m.cfg.MoveRequestedAt = func(context.Context) (time.Time, error) {
		return r.at, nil
	}
	r.m.cfg.MoveFinished = func(_ context.Context, o MoveOutcome) error {
		r.outs = append(r.outs, o)
		return nil
	}
	return r
}

func (r *moveRig) request() { r.at = r.h.clock.Now() }

func (r *moveRig) conflict() {
	r.h.worker.bound, r.h.worker.cluster = true, "other-cluster"
	r.m.init(context.Background())
	r.m.cycle(context.Background())
}

func TestManagerMoveTakesTheLicenseAndAdoptsTheLease(t *testing.T) {
	r := newMoveRig(t)
	r.conflict()
	if st := r.m.Status(); st.State != StateConflict {
		t.Fatalf("before: %+v", st)
	}
	ctx := context.Background()
	r.m.CheckMove(ctx) // no request yet
	if r.h.worker.count("/v1/license/move") != 0 {
		t.Fatal("moved without a request")
	}
	r.h.clock.Advance(time.Minute)
	r.request()
	r.m.CheckMove(ctx)
	if len(r.outs) != 1 || r.outs[0].Result != MoveMoved {
		t.Fatalf("outcomes = %+v", r.outs)
	}
	if st := r.m.Status(); st.State != StateActive {
		t.Fatalf("after: %+v", st)
	}
	if r.h.worker.cluster != "uuid-1" {
		t.Fatalf("worker bound to %q", r.h.worker.cluster)
	}
	if r.h.leases.lease == "" {
		t.Fatal("lease not persisted")
	}
	// The same request is never acted on twice.
	r.m.CheckMove(ctx)
	if r.h.worker.count("/v1/license/move") != 1 || len(r.outs) != 1 {
		t.Fatal("request handled twice")
	}
}

func TestManagerMoveWhenAlreadyHere(t *testing.T) {
	r := newMoveRig(t)
	r.m.init(context.Background())
	r.m.cycle(context.Background())
	r.request()
	r.m.CheckMove(context.Background())
	if len(r.outs) != 1 || r.outs[0].Result != MoveAlreadyHere {
		t.Fatalf("outcomes = %+v", r.outs)
	}
	if st := r.m.Status(); st.State != StateActive {
		t.Fatalf("status = %+v", st)
	}
}

func TestManagerMoveReportsServerRefusals(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   map[string]any
		want   MoveOutcome
	}{
		{"limit", 429, map[string]any{"error": "transfer_limit",
			"retry_after": 3 * 86400},
			MoveOutcome{Result: MoveTransferLimit, RetryAfter: 72 * time.Hour}},
		{"limit without days", 429, map[string]any{"error": "transfer_limit"},
			MoveOutcome{Result: MoveTransferLimit}},
		{"invalid key", 400, map[string]any{"error": "invalid_key"},
			MoveOutcome{Result: MoveInvalidKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newMoveRig(t)
			r.conflict()
			r.h.worker.set(func(p string) (int, map[string]any,
				map[string]string,
			) {
				if p == "/v1/license/move" {
					return tt.status, tt.body, nil
				}
				return 0, nil, nil
			})
			r.request()
			r.m.CheckMove(context.Background())
			if len(r.outs) != 1 || r.outs[0] != tt.want {
				t.Fatalf("outcomes = %+v, want %+v", r.outs, tt.want)
			}
			if st := r.m.Status(); st.State != StateConflict {
				t.Fatalf("a refused move changed the state: %+v", st)
			}
		})
	}
}

func TestManagerMoveRetriesAfterNetworkFailure(t *testing.T) {
	r := newMoveRig(t)
	r.conflict()
	fail := true
	r.h.worker.set(func(p string) (int, map[string]any, map[string]string) {
		if p == "/v1/license/move" && fail {
			return 503, map[string]any{"error": "down"}, nil
		}
		return 0, nil, nil
	})
	r.request()
	ctx := context.Background()
	r.m.CheckMove(ctx)
	if len(r.outs) != 0 {
		t.Fatalf("a transient failure was reported: %+v", r.outs)
	}
	fail = false
	r.h.clock.Advance(time.Minute)
	r.m.CheckMove(ctx)
	if len(r.outs) != 1 || r.outs[0].Result != MoveMoved {
		t.Fatalf("outcomes = %+v", r.outs)
	}
}

func TestManagerMoveDropsAStaleRequest(t *testing.T) {
	r := newMoveRig(t)
	r.conflict()
	r.request()
	r.h.clock.Advance(moveRequestTTL + time.Minute)
	r.m.CheckMove(context.Background())
	if len(r.outs) != 1 || r.outs[0].Result != MoveStale {
		t.Fatalf("outcomes = %+v", r.outs)
	}
	if r.h.worker.count("/v1/license/move") != 0 {
		t.Fatal("a stale request moved the license")
	}
}

func TestManagerMoveAfterDeactivateStaysActive(t *testing.T) {
	r := newMoveRig(t)
	r.m.init(context.Background())
	r.m.cycle(context.Background())
	var deact time.Time
	r.m.cfg.ProcessStart = testNow.Add(-time.Hour)
	r.m.cfg.DeactivatedAt = func(context.Context) (time.Time, error) {
		return deact, nil
	}
	ctx := context.Background()
	r.h.clock.Advance(time.Minute)
	deact = r.h.clock.Now()
	r.m.CheckDeactivated(ctx)
	if st := r.m.Status(); st.State != StatePending {
		t.Fatalf("after deactivate: %+v", st)
	}
	r.h.clock.Advance(time.Minute)
	r.request()
	r.m.CheckMove(ctx)
	if len(r.outs) != 1 || r.outs[0].Result != MoveMoved {
		t.Fatalf("outcomes = %+v", r.outs)
	}
	// The old marker is still in the Secret on the next ticks.
	r.m.CheckDeactivated(ctx)
	if st := r.m.Status(); st.State != StateActive {
		t.Fatalf("after move: %+v", st)
	}
	before := r.h.worker.total()
	r.m.cycle(ctx)
	if r.h.worker.total() == before {
		t.Fatal("manager stayed released after the move")
	}
}
