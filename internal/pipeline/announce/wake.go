package announce

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// noteWake remembers the summary of a wake-up that has just ended, for
// the next digest. Nothing is said while it is on: the pods it started
// are held back, and what still fails when it ends is announced like any
// other problem. A wake-up in which no pod failed is not worth a line.
// One that ended before kwatch looked is not summarised: it may already
// have been, before a restart.
func (c *Collector) noteWake(now time.Time) {
	if c.seenAt.IsZero() {
		c.seenAt = now
	}
	if c.env.History == nil {
		return
	}
	w, ok := kube.LatestWake(c.env.History, now)
	if !ok || w.Remaining(now) > 0 || w.Start.Equal(c.wakeDone) ||
		w.End().Before(c.seenAt) {
		return
	}
	c.wakeDone = w.Start
	report := kube.ReportWake(c.env.History, w)
	if report.Blips == 0 {
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
