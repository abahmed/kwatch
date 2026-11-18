package app

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/health"
)

// lastSeen is the previous session's last liveness stamp, zero on a
// first run or when it is unknown. Start reads it before the restart
// is recorded (measureDowntime).
func (s *startupManager) lastSeen() time.Time {
	if s.gapEnd.IsZero() || s.lastGap <= 0 {
		return time.Time{}
	}
	return s.gapEnd.Add(-s.lastGap)
}

// notWatchingSpans are the stretches between a session stopping and
// the next one starting that end within health.NotWatchingWindow of
// now, oldest first. A start whose previous session left no stop
// record is bounded by its last liveness stamp; one with neither is
// left out, since nothing says when it stopped.
func notWatchingSpans(
	records []restartRecord, now time.Time,
) []health.NotWatchingSpan {
	from := now.Add(-health.NotWatchingWindow)
	var out []health.NotWatchingSpan
	for _, r := range records {
		stopped := r.StoppedAt
		if stopped.IsZero() {
			stopped = r.LastSeen
		}
		if stopped.IsZero() || !r.At.After(stopped) ||
			r.At.Before(from) {
			continue
		}
		out = append(out, health.NotWatchingSpan{From: stopped, To: r.At})
	}
	slices.SortFunc(out, func(a, b health.NotWatchingSpan) int {
		return a.From.Compare(b.From)
	})
	return out
}

// recentNotWatching are the spans that end within the window of now,
// for a report read long after the start.
func recentNotWatching(
	spans []health.NotWatchingSpan, now time.Time,
) []health.NotWatchingSpan {
	from := now.Add(-health.NotWatchingWindow)
	out := []health.NotWatchingSpan{}
	for _, s := range spans {
		if !s.To.Before(from) {
			out = append(out, s)
		}
	}
	return out
}
