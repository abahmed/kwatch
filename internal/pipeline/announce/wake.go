package announce

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// wakeLineMaxDelay bounds how long after a wake-up ended a digest still
// summarises it: a digest normally goes DigestWindow after the end, and
// a line about a short spell an hour or more before reads as news it
// is not.
const wakeLineMaxDelay = DigestWindow + kube.WakeQuiet

// noteWake remembers the summary of a wake-up that has just ended, for
// the next digest. Nothing is said while it is on: the pods it started
// are held back, and what still fails when it ends is announced like any
// other problem. A wake-up in which no pod failed is not worth a line,
// nor is one in which no workload started from zero (only nodes
// joined): the line counts started workloads. One that began before
// kwatch looked is not summarised: kwatch did not see it start, and it
// may already have been summarised before a restart.
func (c *Collector) noteWake(now time.Time) {
	if c.seenAt.IsZero() {
		c.seenAt = now
	}
	if c.env.History == nil {
		return
	}
	w, ok := kube.LatestWake(c.env.History, now)
	if !ok || w.Remaining(now) > 0 || w.Start.Equal(c.wakeDone) ||
		w.Start.Before(c.seenAt) {
		return
	}
	c.wakeDone = w.Start
	report := kube.ReportWake(c.env.History, w)
	if report.Blips == 0 || len(w.Workloads) == 0 {
		return
	}
	c.wake = &compose.WakeLine{
		Started: len(w.Workloads), From: w.Start, To: w.Last,
		Blips: report.Blips, Failing: report.Failing,
	}
	if c.Low.Since.IsZero() {
		c.Low.Since = now
	}
}

// freshWake is the wake line a digest sent at now may carry: nil when
// there is none or when the wake-up ended more than wakeLineMaxDelay
// before now.
func freshWake(line *compose.WakeLine, now time.Time) *compose.WakeLine {
	if line == nil {
		return nil
	}
	end := kube.Wake{Start: line.From, Last: line.To}.End()
	if now.Sub(end) > wakeLineMaxDelay {
		return nil
	}
	return line
}
