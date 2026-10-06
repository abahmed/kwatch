package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/metrics"
)

func TestOpenStoreResetsUnreadableFileAndReportsHealth(t *testing.T) {
	dir := useStateEnv(t)
	path := filepath.Join(dir, "state.db")
	require.NoError(t, os.WriteFile(path, []byte("garbage"), 0o600))
	deps := epochDeps(leaseWithTransitions(ptr.To(int32(1))))
	deps.healthServer = health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})

	s, err := openStore(context.Background(), deps)
	require.NoError(t, err)
	defer closeStore(s)
	reportStoreReset(deps, s)

	reset, ok := s.Reset()
	require.True(t, ok)
	require.Equal(t, "unreadable", reset.Reason)
	leftovers, err := filepath.Glob(path + ".*")
	require.NoError(t, err)
	require.Equal(t, []string{path + ".corrupt"}, leftovers,
		"the old file is kept aside")
	status := deps.healthServer.ComponentStatuses()["state-store"]
	require.Equal(t, "degraded", status.State)
	require.Equal(t, "storage_reset", status.Reason)
	require.True(t, status.Available, "a reset never blocks readiness")
	require.NoError(t, diskState{store: s}.put("k", "v"))
}

func TestReportStoreResetIsSilentForHealthyFile(t *testing.T) {
	deps := componentDeps()

	reportStoreReset(deps, openTestStore(t))

	_, reported := deps.healthServer.ComponentStatuses()["state-store"]
	require.False(t, reported)
}

func TestRunCompactorStopsOnCancel(t *testing.T) {
	s := openTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	run := runCompactor(s, newStorageMetrics(&metrics.Registry{}))
	go func() { done <- run(ctx) }()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("compactor did not stop after cancellation")
	}
	require.NoError(t, diskState{store: s}.put("k", "v"),
		"the store is still usable after the compactor stops")
}

// A replica that does not hold the Lease must leave an unreadable file
// alone: the leader may still need it, and deleting it is the leader's
// decision.
func TestOpenStoreLeavesUnreadableFileWhenNotLeaseHolder(t *testing.T) {
	dir := useStateEnv(t)
	path := filepath.Join(dir, "state.db")
	require.NoError(t, os.WriteFile(path, []byte("garbage"), 0o600))
	deps := epochDeps(leaseHeldBy("kwatch-1", nil))

	_, err := openStore(context.Background(), deps)

	require.ErrorIs(t, err, errNotLeaseHolder)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "garbage", string(content), "file was repaired early")
}
