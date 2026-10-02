package app

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func TestStartupManagerAnnouncesUpgradeAfterDowntime(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	store := &testStateStore{
		firstRun:  false,
		version:   "old-version",
		lastSeen:  now.Add(-10 * time.Minute),
		clusterID: "cluster-1",
	}
	manager := newStartupManagerWithRuntime(
		store,
		config.RuntimeConfigFor(&config.Config{}),
		clock.RealClock{},
	)
	manager.now = func() time.Time { return now }
	result, err := manager.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if result.ClusterID != "cluster-1" || result.CurrentVersion != "dev" {
		t.Fatalf("startup result = %+v", result)
	}
	msg, ok := manager.StartupMessage()
	want := "🟠 kwatch dev started; nothing was monitored from 11:50 to " +
		"12:00 UTC (10m), so anything that broke then went unreported."
	if !ok || msg != want {
		t.Fatalf("startup message = %q (%v), want %q", msg, ok, want)
	}
}

type testStateStore struct {
	firstRun  bool
	version   string
	lastSeen  time.Time
	clusterID string
}

func (s *testStateStore) EnsureClusterID(context.Context) (string, error) {
	return s.clusterID, nil
}

func (s *testStateStore) IsFirstRun(context.Context) (bool, error) {
	return s.firstRun, nil
}

func (s *testStateStore) GetStoredVersion(context.Context) (string, error) {
	return s.version, nil
}

func (s *testStateStore) MarkAsInitialized(
	context.Context, string, string,
) error {
	return nil
}

func (s *testStateStore) GetLastSeen(context.Context) (time.Time, error) {
	return s.lastSeen, nil
}

func (s *testStateStore) SetLastSeen(
	_ context.Context, value time.Time,
) error {
	s.lastSeen = value
	return nil
}

// resetStateStore is a state file that was replaced at open.
type resetStateStore struct{ testStateStore }

func (resetStateStore) StoreWasReset() bool { return true }

// A replaced state file is always announced, and the message says why
// old problems are about to be announced again.
func TestStartupManagerAnnouncesStateReset(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	store := &resetStateStore{testStateStore{version: "dev",
		clusterID: "cluster-1"}}
	manager := newStartupManagerWithRuntime(store,
		config.RuntimeConfigFor(&config.Config{}), clock.RealClock{})
	manager.now = func() time.Time { return now }

	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	msg, ok := manager.StartupMessage()
	want := "🟠 kwatch dev started with fresh state: earlier incident " +
		"history was reset, so open problems will be announced again."
	if !ok || msg != want {
		t.Fatalf("startup message = %q (%v), want %q", msg, ok, want)
	}
}
