package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/health"
)

// startAt runs one startup over disk at now.
func startAt(
	t *testing.T, disk diskState, now time.Time,
) *startupManager {
	t.Helper()
	sm := newStartupManager(disk, true,
		clock.Func(func() time.Time { return now }))
	_, err := sm.Start(context.Background())
	require.NoError(t, err)
	return sm
}

// Each start records how the previous session ended: its own stop
// record when it wrote one, else the classified reason of a crash.
func TestStartupRecordsHowThePreviousSessionEnded(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("NODE_NAME", "")
	disk := diskState{store: openTestStore(t)}
	t0 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

	first := startAt(t, disk, t0)
	require.Nil(t, first.lastStop, "a first run follows nothing")
	require.Equal(t, "", first.gap().restart.Reason)
	first.RecordAlive(context.Background())
	first.EndSession(context.Background(), "leader_handoff")

	second := startAt(t, disk, t0.Add(40*time.Second))
	require.NotNil(t, second.lastStop)
	assert.Equal(t, "leader_handoff", second.lastStop.Reason)
	assert.Equal(t, t0, second.lastStop.StoppedAt)
	notice := second.gap()
	assert.Equal(t, "leader_handoff", notice.restart.Reason)
	assert.Equal(t, 40*time.Second, notice.restart.Down)
	assert.Equal(t, t0.Add(40*time.Second), notice.self.startedAt)

	// No stop record: the session crashed.
	third := startAt(t, disk, t0.Add(2*time.Minute))
	assert.Equal(t, "internal_failure", third.lastStop.Reason)
	assert.True(t, third.lastStop.StoppedAt.IsZero())
	assert.Equal(t, []health.SelfStop{
		{At: t0.Add(2 * time.Minute), Reason: "internal_failure"},
		{At: t0.Add(40 * time.Second), Reason: "leader_handoff",
			StoppedAt: t0},
	}, third.restarts, "newest first")
}

func TestRecentRestartsKeepsTheLastDay(t *testing.T) {
	now := time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)
	records := []restartRecord{
		{At: now.Add(-25 * time.Hour), Reason: "oom_killed"},
		{At: now.Add(-time.Hour), Reason: "graceful_shutdown"},
	}

	got := recentRestarts(records, now)

	require.Len(t, got, 1)
	assert.Equal(t, "graceful_shutdown", got[0].Reason)
}

func TestStopReasonReadsTheStopRecordFirst(t *testing.T) {
	assert.Equal(t, "", stopReason(runtimeSession{}, "x"))
	assert.Equal(t, "graceful_shutdown", stopReason(runtimeSession{
		SessionID: "s", EndReason: "graceful_shutdown"}, ""))
	assert.Equal(t, "oom_killed",
		stopReason(runtimeSession{SessionID: "s"}, "oom_killed"))
	assert.Equal(t, "unknown", stopReason(runtimeSession{
		SessionID: "s", EndedAt: time.Now()}, ""))
}

func TestActiveShutdownEndReason(t *testing.T) {
	assert.Equal(t, "graceful_shutdown", activeShutdown{}.endReason())
	assert.Equal(t, "leader_handoff",
		activeShutdown{leadershipLost: true}.endReason())
	assert.Equal(t, "internal_failure",
		activeShutdown{failed: true, leadershipLost: true}.endReason())
}

// The restart history is bounded.
func TestRestartHistoryKeepsAtMostMaxRecords(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	t0 := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	for i := range maxRestartRecords + 3 {
		sm := startAt(t, disk, t0.Add(time.Duration(i)*time.Minute))
		sm.EndSession(context.Background(), "graceful_shutdown")
	}

	records, err := disk.GetRestarts(context.Background())

	require.NoError(t, err)
	assert.Len(t, records, maxRestartRecords)
}
