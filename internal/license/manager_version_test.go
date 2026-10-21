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
