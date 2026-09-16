package startup

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func TestStartReturnsExplicitUpgradeAndDowntimeDecision(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	store := &testStateStore{
		firstRun:  false,
		version:   "old-version",
		lastSeen:  now.Add(-10 * time.Minute),
		clusterID: "cluster-1",
	}
	manager := NewStartupManagerWithRuntime(
		store,
		config.RuntimeConfigFor(&config.Config{}),
		clock.RealClock{},
	)
	manager.now = func() time.Time { return now }
	result, err := manager.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if !result.Upgrade || result.FirstRun {
		t.Fatalf("unexpected startup flags: %+v", result)
	}
	if result.Downtime != 10*time.Minute || !result.ShouldNotify {
		t.Fatalf("unexpected startup decision: %+v", result)
	}
	if result.ClusterID != "cluster-1" {
		t.Fatalf("cluster ID = %q, want cluster-1", result.ClusterID)
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

func (s *testStateStore) GetStoredVersion(context.Context) string {
	return s.version
}

func (s *testStateStore) MarkAsInitialized(
	context.Context, string, string,
) error {
	return nil
}

func (s *testStateStore) GetLastSeen(context.Context) time.Time {
	return s.lastSeen
}

func (s *testStateStore) SetLastSeen(
	_ context.Context, value time.Time,
) error {
	s.lastSeen = value
	return nil
}
