package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/incident"
)

// notWatching is when kwatch was not running in the last week (the
// restart records), oldest first; nil without a session.
func (s *Server) notWatching() []health.NotWatchingSpan {
	self, ok := s.selfReport()
	if !ok {
		return nil
	}
	return self.NotWatching
}

// addOffline sets, on each day of the week (in zone, today last), how
// much of it has passed and how much of that kwatch was not running.
func addOffline(
	days []InsightDay, spans []health.NotWatchingSpan,
	now time.Time, zone *time.Location,
) {
	if zone == nil {
		zone = time.UTC
	}
	w := newTrendWeek(now, zone)
	for i := range days {
		from := w.start.AddDate(0, 0, i)
		to := minTime(w.start.AddDate(0, 0, i+1), now)
		if !to.After(from) {
			continue
		}
		days[i].Seconds = int(to.Sub(from) / time.Second)
		days[i].OffSeconds = int(overlap(spans, from, to) / time.Second)
	}
}

// overlap is how much of [from, to) the spans cover. Spans do not
// overlap each other: a session starts after the previous one stopped.
func overlap(
	spans []health.NotWatchingSpan, from, to time.Time,
) time.Duration {
	var total time.Duration
	for _, s := range spans {
		start, end := maxTime(s.From, from), minTime(s.To, to)
		if end.After(start) {
			total += end.Sub(start)
		}
	}
	return total
}

// spansWithin are the spans that overlap [from, to), cut to it.
func spansWithin(
	spans []health.NotWatchingSpan, from, to time.Time,
) []health.NotWatchingSpan {
	var out []health.NotWatchingSpan
	for _, s := range spans {
		start, end := maxTime(s.From, from), minTime(s.To, to)
		if end.After(start) {
			out = append(out, health.NotWatchingSpan{From: start, To: end})
		}
	}
	return out
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// issueNotWatching are the spans while p was open: from its first
// opening or its first timeline event, whichever is earlier, to its
// resolve, or now while it is not resolved.
func issueNotWatching(
	spans []health.NotWatchingSpan, p incident.Incident, now time.Time,
) []health.NotWatchingSpan {
	from := p.Opened
	for _, at := range p.Occurrences {
		from = minTime(from, at)
	}
	if len(p.Timeline) > 0 && !p.Timeline[0].At.IsZero() {
		from = minTime(from, p.Timeline[0].At)
	}
	to := now
	if p.State == incident.Resolved && !p.Resolved.IsZero() {
		to = p.Resolved
	}
	if from.IsZero() {
		return nil
	}
	return spansWithin(spans, from, to)
}
