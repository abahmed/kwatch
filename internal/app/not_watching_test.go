package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
)

// A night off is a span from the stop to the next start; a crash that
// left no stop record is bounded by its last liveness stamp.
func TestStartupRecordsWhenKwatchWasNotRunning(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("NODE_NAME", "")
	disk := diskState{store: openTestStore(t)}
	t0 := time.Date(2026, 10, 1, 22, 0, 0, 0, time.UTC)

	first := startAt(t, disk, t0)
	first.RecordAlive(context.Background())
	first.EndSession(context.Background(), "graceful_shutdown")
	morning := t0.Add(8 * time.Hour)
	second := startAt(t, disk, morning)
	second.RecordAlive(context.Background())
	// No stop record: the node went away.
	third := startAt(t, disk, morning.Add(3*time.Hour))

	assert.Equal(t, []health.NotWatchingSpan{
		{From: t0, To: morning},
		{From: morning, To: morning.Add(3 * time.Hour)},
	}, third.gap().self.notWatching)
}

func TestNotWatchingSpansKeepTheWeekAndSkipUnknownStops(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	records := []restartRecord{
		{At: now.Add(-9 * 24 * time.Hour),
			StoppedAt: now.Add(-9*24*time.Hour - time.Hour)},
		{At: now.Add(-2 * time.Hour)},
		{At: now.Add(-time.Hour), StoppedAt: now.Add(-90 * time.Minute)},
		{At: now.Add(-3 * time.Hour), LastSeen: now.Add(-5 * time.Hour)},
	}

	got := notWatchingSpans(records, now)

	require.Equal(t, []health.NotWatchingSpan{
		{From: now.Add(-5 * time.Hour), To: now.Add(-3 * time.Hour)},
		{From: now.Add(-90 * time.Minute), To: now.Add(-time.Hour)},
	}, got, "oldest first; too old and unknown stops left out")
	later := recentNotWatching(got, now.Add(8*24*time.Hour-
		2*time.Hour))
	require.Len(t, later, 1, "a span leaves once its end is a week old")
	require.NotNil(t, recentNotWatching(nil, now), "never null in JSON")
}
