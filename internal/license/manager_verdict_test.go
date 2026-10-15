package license

import (
	"context"
	"testing"
	"time"
)

func TestManagerVerdicts(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   map[string]any
		want   State
	}{
		{"conflict", 409,
			map[string]any{"error": "bound_to_other_cluster",
				"bound_since": 5}, StateConflict},
		{"revoked", 403, map[string]any{"error": "revoked"}, StateRevoked},
		{"suspended", 403, map[string]any{"error": "suspended"}, StateRevoked},
		{"key replaced", 403, map[string]any{"error": "key_replaced"}, StateInvalid},
		{"trial used", 403,
			map[string]any{"error": "trial_already_used"}, StateInvalid},
		{"server expired", 403, map[string]any{"error": "expired"}, StateExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.worker.set(func(string) (int, map[string]any, map[string]string) {
				return tt.status, tt.body, nil
			})
			d := h.run()
			if st := h.m.Status(); st.State != tt.want || st.Reason == "" {
				t.Fatalf("status = %+v", st)
			}
			if d != 24*time.Hour {
				t.Fatalf("delay = %v", d)
			}
			// Recovery clears the verdict.
			h.worker.set(nil)
			h.m.cycle(context.Background())
			if st := h.m.Status(); st.State != StateActive {
				t.Fatalf("after recovery: %+v", st)
			}
		})
	}
}

func TestManagerInstanceSuperseded(t *testing.T) {
	h := newHarness(t)
	h.run()
	h.clock.Advance(25 * time.Hour)
	h.worker.set(func(p string) (int, map[string]any, map[string]string) {
		if p == "/v1/license/renew" {
			return 409, map[string]any{"error": "instance_superseded"}, nil
		}
		return 0, nil, nil
	})
	h.m.cycle(context.Background())
	if st := h.m.Status(); st.State != StateConflict {
		t.Fatalf("status = %+v", st)
	}
	// Retry renews (does not re-activate and steal the binding).
	if n := h.worker.count("/v1/license/activate"); n != 1 {
		t.Fatalf("activates = %d", n)
	}
	h.m.cycle(context.Background())
	if n := h.worker.count("/v1/license/activate"); n != 1 {
		t.Fatalf("activates after retry = %d", n)
	}
}

func TestManagerOfflineGraceExpiry(t *testing.T) {
	h := newHarness(t)
	h.run()
	h.worker.srv.Close() // now unreachable

	m := h.newManager(h.key) // restart with the cached lease
	h.m = m
	m.init(context.Background())
	if st := m.Status(); st.State != StateActive {
		t.Fatalf("cached: %+v", st)
	}
	want := []time.Duration{
		time.Minute, 5 * time.Minute, 15 * time.Minute,
		30 * time.Minute, time.Hour, time.Hour,
	}
	for i, w := range want {
		if d := m.cycle(context.Background()); d != w {
			t.Fatalf("backoff[%d] = %v, want %v", i, d, w)
		}
	}
	if st := m.Status(); st.State != StateActive {
		t.Fatalf("after failures: %+v", st)
	}
	h.clock.Advance(5 * 24 * time.Hour)
	if st := m.Status(); st.State != StateGrace || !st.State.ProEnabled() {
		t.Fatalf("grace: %+v", st)
	}
	h.clock.Advance(3 * 24 * time.Hour)
	if st := m.Status(); st.State != StateExpired {
		t.Fatalf("expired: %+v", st)
	}
}

func TestManagerRenewSigned(t *testing.T) {
	h := newHarness(t)
	h.run()
	first := h.leases.lease
	h.clock.Advance(25 * time.Hour)
	h.m.cycle(context.Background())
	w := h.worker
	if w.count("/v1/license/renew") != 1 || w.badSigs != 0 {
		t.Fatalf("renews=%d badSigs=%d", w.count("/v1/license/renew"), w.badSigs)
	}
	if w.count("/v1/license/activate") != 1 {
		t.Fatal("renew should not re-activate")
	}
	if h.leases.lease == first {
		t.Fatal("lease not refreshed")
	}
	if w.lastRenew["lease_iat"].(float64) != float64(testNow.Unix()) ||
		w.lastRenew["lid"] != "lic_abcdef1234" {
		t.Fatalf("renew body = %v", w.lastRenew)
	}
	if st := h.m.Status(); st.State != StateActive {
		t.Fatalf("status = %+v", st)
	}
}

func TestManagerRenew401Reactivates(t *testing.T) {
	h := newHarness(t)
	h.run()
	h.clock.Advance(25 * time.Hour)
	h.worker.mu.Lock()
	h.worker.bound = false // binding transferred away
	h.worker.mu.Unlock()
	h.m.cycle(context.Background())
	if h.worker.count("/v1/license/renew") != 1 ||
		h.worker.count("/v1/license/activate") != 2 {
		t.Fatalf("renew=%d activate=%d",
			h.worker.count("/v1/license/renew"),
			h.worker.count("/v1/license/activate"))
	}
	if st := h.m.Status(); st.State != StateActive {
		t.Fatalf("status = %+v", st)
	}
}

func TestManagerRetryAfter(t *testing.T) {
	h := newHarness(t)
	h.worker.set(func(string) (int, map[string]any, map[string]string) {
		return 429, map[string]any{"error": "rate_limited"},
			map[string]string{"Retry-After": "7200"}
	})
	if d := h.run(); d != 2*time.Hour {
		t.Fatalf("delay = %v, want 2h", d)
	}
	h.worker.set(func(string) (int, map[string]any, map[string]string) {
		return 429, map[string]any{"error": "rate_limited"},
			map[string]string{"Retry-After": "5"}
	})
	// second failure -> 5m backoff
	if d := h.m.cycle(context.Background()); d != 5*time.Minute {
		t.Fatalf("delay = %v, want 5m", d)
	}
}

func TestManagerClockRollback(t *testing.T) {
	h := newHarness(t)
	h.run()
	h.clock.Advance(-3 * time.Hour)
	st := h.m.Status()
	if st.State != StateGrace || st.Reason == "" {
		t.Fatalf("status = %+v", st)
	}
	// A restart keeps the persisted guard.
	m := h.newManager(h.key)
	m.init(context.Background())
	if st := m.Status(); st.State != StateGrace {
		t.Fatalf("after restart: %+v", st)
	}
}
