package license

import (
	"context"
	"errors"
	"testing"
	"time"
)

// memVersionLeaseStore is a LeaseStore that also records the version.
type memVersionLeaseStore struct {
	*memLeaseStore
	version string
	loadErr error
	saves   int
	attempt RenewAttempt
}

func (s *memVersionLeaseStore) LoadRenewAttempt() (RenewAttempt, error) {
	return s.attempt, nil
}

func (s *memVersionLeaseStore) SaveRenewAttempt(a RenewAttempt) error {
	s.attempt = a
	return nil
}

func (s *memVersionLeaseStore) LoadReportedVersion() (string, error) {
	return s.version, s.loadErr
}

func (s *memVersionLeaseStore) SaveReportedVersion(v string) error {
	s.version = v
	s.saves++
	return nil
}

var _ VersionStore = (*memVersionLeaseStore)(nil)

// newVersionHarness activates once at v1.2.3 so a lease and the
// reported version are on record.
func newVersionHarness(t *testing.T) (*harness, *memVersionLeaseStore) {
	h := newHarness(t)
	store := &memVersionLeaseStore{memLeaseStore: h.leases}
	h.m = h.restart(store, "v1.2.3")
	if d := h.m.cycle(context.Background()); d != renewInterval {
		t.Fatalf("activation delay = %v", d)
	}
	if store.version != "v1.2.3" {
		t.Fatalf("recorded version = %q", store.version)
	}
	return h, store
}

// restart builds a new manager over the same stores, as a pod restart
// does, and loads its cached state.
func (h *harness) restart(store LeaseStore, version string) *Manager {
	m := h.newManager(h.key)
	m.cfg.LeaseStore, m.cfg.Version = store, version
	m.init(context.Background())
	return m
}

func TestManagerSameVersionKeepsTheRenewSchedule(t *testing.T) {
	h, store := newVersionHarness(t)
	h.clock.Advance(time.Hour)
	m := h.restart(store, "v1.2.3")
	if d := m.initialDelay(context.Background()); d != 23*time.Hour {
		t.Fatalf("initial delay = %v, want 23h", d)
	}
}

func TestManagerNewVersionRenewsAtOnce(t *testing.T) {
	h, store := newVersionHarness(t)
	h.clock.Advance(time.Hour)
	m := h.restart(store, "v1.3.0")
	ctx := context.Background()
	if d := m.initialDelay(ctx); d != 0 {
		t.Fatalf("initial delay = %v, want 0", d)
	}
	m.cycle(ctx)
	if n := h.worker.count("/license/renew"); n != 1 {
		t.Fatalf("renewals = %d, want 1", n)
	}
	if v := h.worker.lastRenew["kwatch_version"]; v != "v1.3.0" {
		t.Fatalf("renew sent kwatch_version %v", v)
	}
	if store.version != "v1.3.0" {
		t.Fatalf("recorded version = %q", store.version)
	}
}

func TestManagerNoRecordedVersionRenewsAtOnce(t *testing.T) {
	h, store := newVersionHarness(t)
	store.version = ""
	m := h.restart(store, "v1.2.3")
	if d := m.initialDelay(context.Background()); d != 0 {
		t.Fatalf("initial delay = %v, want 0", d)
	}
}

func TestManagerCrashLoopAtOneVersionRenewsOnce(t *testing.T) {
	h, store := newVersionHarness(t)
	ctx := context.Background()
	for range 5 {
		h.clock.Advance(time.Minute)
		m := h.restart(store, "v1.3.0")
		if m.initialDelay(ctx) == 0 {
			m.cycle(ctx)
		}
	}
	if n := h.worker.count("/license/renew"); n != 1 {
		t.Fatalf("renewals = %d, want 1 across the crash loop", n)
	}
	if store.saves != 2 {
		t.Fatalf("version saves = %d, want 2", store.saves)
	}
}

func TestManagerUnreadableVersionKeepsTheRenewSchedule(t *testing.T) {
	h, store := newVersionHarness(t)
	store.loadErr = errors.New("disk")
	m := h.restart(store, "v1.3.0")
	if d := m.initialDelay(context.Background()); d != renewInterval {
		t.Fatalf("initial delay = %v, want the interval", d)
	}
}

func TestManagerStoreWithoutVersionKeepsTheRenewSchedule(t *testing.T) {
	h := newHarness(t)
	h.run()
	m := h.restart(h.leases, "v9.9.9")
	if d := m.initialDelay(context.Background()); d != renewInterval {
		t.Fatalf("initial delay = %v, want the interval", d)
	}
}

func TestManagerFailedRenewDoesNotRecordTheVersion(t *testing.T) {
	h, store := newVersionHarness(t)
	h.worker.set(func(string) (int, map[string]any, map[string]string) {
		return 503, map[string]any{"error": "unavailable"}, nil
	})
	m := h.restart(store, "v1.3.0")
	m.cycle(context.Background())
	if store.version != "v1.2.3" {
		t.Fatalf("recorded version = %q after a failed renew", store.version)
	}
}

func TestManagerCrashLoopWithServerDownSpacesEarlyRenewals(t *testing.T) {
	h, store := newVersionHarness(t)
	down := true
	h.worker.set(func(string) (int, map[string]any, map[string]string) {
		if down {
			return 503, map[string]any{"error": "unavailable"}, nil
		}
		return 0, nil, nil
	})
	ctx := context.Background()
	restart := func() time.Duration {
		m := h.restart(store, "v1.3.0")
		d := m.initialDelay(ctx)
		if d == 0 {
			m.cycle(ctx)
		}
		return d
	}
	for range 10 {
		h.clock.Advance(5 * time.Second)
		if d := restart(); d > earlyRenewBase {
			t.Fatalf("delay %v exceeds the remaining window", d)
		}
	}
	if n := h.worker.count("/license/renew"); n > 2 {
		t.Fatalf("renewals = %d across 10 rapid restarts, want <= 2", n)
	}
	before := h.worker.count("/license/renew")
	h.clock.Advance(earlyRenewBase)
	restart()
	if n := h.worker.count("/license/renew"); n != before+1 {
		t.Fatalf("no retry after the window: %d -> %d", before, n)
	}
	if store.attempt.Count != 2 {
		t.Fatalf("attempt count = %d, want 2", store.attempt.Count)
	}
	down = false
	h.clock.Advance(earlyRenewCap)
	restart()
	if store.version != "v1.3.0" || store.attempt != (RenewAttempt{}) {
		t.Fatalf("after success version=%q attempt=%+v",
			store.version, store.attempt)
	}
}

func TestRenewAttemptWindowGrowsToItsCap(t *testing.T) {
	want := map[int]time.Duration{1: time.Minute, 2: 2 * time.Minute,
		3: 4 * time.Minute, 6: earlyRenewCap, 40: earlyRenewCap}
	for n, w := range want {
		if got := (RenewAttempt{Count: n}).window(); got != w {
			t.Fatalf("window(%d) = %v, want %v", n, got, w)
		}
	}
}
