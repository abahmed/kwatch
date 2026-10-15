package license

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagerClockGuardFollowsLocalClockAndPersistsHourly(t *testing.T) {
	h := newHarness(t)
	h.run()
	saves := h.leases.saves
	h.m.Status()
	h.clock.Advance(10 * time.Minute)
	h.m.Status()
	if h.leases.saves != saves {
		t.Fatalf("saved %d times within the hour", h.leases.saves-saves)
	}
	h.clock.Advance(2 * time.Hour)
	h.m.Status()
	if h.leases.saves != saves+1 {
		t.Fatalf("saves = %d, want one hourly write", h.leases.saves-saves)
	}
	if !h.leases.clock.MaxSeen.Equal(h.clock.Now()) {
		t.Fatalf("persisted MaxSeen %v, want %v",
			h.leases.clock.MaxSeen, h.clock.Now())
	}
}

func TestManagerRolledBackClockCannotRevive(t *testing.T) {
	h := newHarness(t)
	h.run()
	h.clock.Advance(8 * 24 * time.Hour) // lease 3d + grace 4d is over
	if st := h.m.Status(); st.State != StateExpired {
		t.Fatalf("expired: %+v", st)
	}
	h.clock.Advance(-7 * 24 * time.Hour) // clock set back into the lease
	if st := h.m.Status(); st.State.ProEnabled() {
		t.Fatalf("rolled back clock revived the lease: %+v", st)
	}
	m := h.newManager(h.key) // restart: the guard comes from the store
	m.init(context.Background())
	if st := m.Status(); st.State.ProEnabled() {
		t.Fatalf("restart revived the lease: %+v", st)
	}
}

func TestManagerBodySrvTimeIsNotTrusted(t *testing.T) {
	h := newHarness(t)
	far := h.clock.Now().Add(10 * 365 * 24 * time.Hour).Unix()
	h.worker.set(func(string) (int, map[string]any, map[string]string) {
		return 403, map[string]any{"error": "revoked", "srv_time": far}, nil
	})
	h.run()
	if got := h.m.clock.MaxSeen; got.Unix() >= far {
		t.Fatalf("error srv_time moved the guard to %v", got)
	}
	// A 200 whose body srv_time is huge but whose lease is normal.
	h2 := newHarness(t)
	h2.run()
	h2.clock.Advance(25 * time.Hour)
	h2.worker.set(func(p string) (int, map[string]any, map[string]string) {
		if p != "/v1/license/renew" {
			return 0, nil, nil
		}
		lease := h2.worker.lease("lic_abcdef1234")
		return 200, map[string]any{"lease": lease, "srv_time": far}, nil
	})
	h2.m.cycle(context.Background())
	if n := h2.worker.count("/v1/license/renew"); n != 1 {
		t.Fatalf("renews = %d", n)
	}
	if got := h2.m.clock.MaxSeen; got.Unix() >= far {
		t.Fatalf("body srv_time moved the guard to %v", got)
	}
}

func TestManagerSignedLeaseSrvTimeMovesGuard(t *testing.T) {
	h := newHarness(t)
	h.worker.clock = &fakeClock{t: testNow.Add(3 * time.Hour)} // server ahead
	h.run()
	if got := h.m.clock.MaxSeen; got.Before(testNow.Add(3 * time.Hour)) {
		t.Fatalf("lease srv_time not observed: %v", got)
	}
	if st := h.m.Status(); !st.State.ProEnabled() {
		t.Fatalf("status = %+v", st)
	}
}

func TestManagerKeepsRenewingExpiredKeyWithExtendingLease(t *testing.T) {
	h := newHarness(t)
	_, priv := testEnv()
	h.key = SignKey(testKey(testNow.Add(10*24*time.Hour)), priv)
	h.m = h.newManager(h.key)
	h.run()
	for i := 0; i < 14; i++ { // well past the key's own expiry
		h.clock.Advance(24 * time.Hour)
		h.m.cycle(context.Background())
	}
	if st := h.m.Status(); st.State != StateActive {
		t.Fatalf("status = %+v", st)
	}
	if n := h.worker.count("/v1/license/renew"); n < 13 {
		t.Fatalf("renews = %d: stopped renewing at key expiry", n)
	}
	if st := h.m.Status(); st.ExpiresAt.Before(h.clock.Now()) {
		t.Fatalf("ExpiresAt %v is not the lease's lic_exp", st.ExpiresAt)
	}
}

func TestManagerExpiredKeyWithoutLeaseStaysQuiet(t *testing.T) {
	h := newHarness(t)
	_, priv := testEnv()
	h.m = h.newManager(SignKey(testKey(testNow.Add(-time.Hour)), priv))
	h.run()
	if n := h.worker.total(); n != 0 {
		t.Fatalf("requests = %d for an expired key", n)
	}
	if st := h.m.Status(); st.State != StateExpired {
		t.Fatalf("status = %+v", st)
	}
}

func TestManagerClusterIDFromLiveUIDAndPendingWithoutIt(t *testing.T) {
	h := newHarness(t)
	var fail atomic.Bool
	fail.Store(true)
	m := NewManager(Config{
		Key: h.key, Verifier: h.m.cfg.Verifier, Client: h.m.cfg.Client,
		InstanceStore: h.inst, LeaseStore: h.leases,
		ClusterUUIDFrom: func(uid string) string { return "derived-" + uid },
		KubeSystemUID: func(context.Context) (string, error) {
			if fail.Load() {
				return "", errors.New("api down")
			}
			return "kube-uid", nil
		},
		Version: "v1", Now: h.clock.Now, Sleep: h.m.cfg.Sleep,
		Jitter: func(time.Duration) time.Duration { return 0 },
	})
	ctx := context.Background()
	m.init(ctx)
	if st := m.Status(); st.State != StatePending || st.State.ProEnabled() {
		t.Fatalf("status = %+v, want pending", st)
	}
	for _, want := range []time.Duration{time.Minute, 5 * time.Minute} {
		if d := m.cycle(ctx); d != want {
			t.Fatalf("backoff = %v, want %v", d, want)
		}
	}
	if n := h.worker.total(); n != 0 {
		t.Fatalf("requests = %d without a cluster identity", n)
	}
	fail.Store(false)
	m.cycle(ctx)
	if got := h.worker.lastActive["cluster_uuid"]; got != "derived-kube-uid" {
		t.Fatalf("cluster_uuid = %v", got)
	}
	if st := m.Status(); st.State != StateActive {
		t.Fatalf("status = %+v", st)
	}
}

func TestManagerDeactivationMarker(t *testing.T) {
	h := newHarness(t)
	h.run()
	var at time.Time
	marker := func(context.Context) (time.Time, error) { return at, nil }
	cfg := func(start time.Time) *Manager {
		m := h.newManager(h.key)
		m.cfg.DeactivatedAt, m.cfg.ProcessStart = marker, start
		return m
	}
	ctx := context.Background()

	// No marker: nothing changes.
	m := cfg(testNow.Add(-time.Hour))
	m.init(ctx)
	m.CheckDeactivated(ctx)
	if st := m.Status(); st.State != StateActive {
		t.Fatalf("no marker: %+v", st)
	}

	// Marker written while this process runs: lease dropped, no contact.
	at = h.clock.Now().Add(time.Second)
	m.CheckDeactivated(ctx)
	if st := m.Status(); st.State.ProEnabled() || st.State != StatePending {
		t.Fatalf("after marker: %+v", st)
	}
	before := h.worker.total()
	m.cycle(ctx)
	if h.worker.total() != before {
		t.Fatal("a released manager contacted the server")
	}
	if h.leases.lease != "" {
		t.Fatal("dropped lease still persisted")
	}

	// A new generation in the same process stays released.
	again := cfg(testNow.Add(-time.Hour))
	again.init(ctx)
	again.cycle(ctx)
	if h.worker.total() != before {
		t.Fatal("restarted session re-activated")
	}

	// A marker older than the process (pod restarted since) activates.
	fresh := cfg(h.clock.Now().Add(time.Hour))
	fresh.init(ctx)
	fresh.cycle(ctx)
	if st := fresh.Status(); st.State != StateActive {
		t.Fatalf("after pod restart: %+v", st)
	}
}
