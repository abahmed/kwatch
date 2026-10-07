package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// withWake tells an announcement that the cluster was waking up when the
// problem began, so a failure that outlasted the wake-up reads as what it
// is. Only a problem that began after the wake-up did is told so.
func (a *announcer) withWake(
	d incident.Decision, at time.Time,
) incident.Decision {
	if a.history == nil || d.Action != incident.Announce {
		return d
	}
	w, ok := kube.ContextWake(a.history, at)
	if !ok || d.Incident.Opened.Before(w.Start) {
		return d
	}
	d.Facts.Wake = &incident.WakeContext{Started: len(w.Workloads),
		From: w.Start, To: w.Last, Ongoing: w.Remaining(at) > 0}
	return d
}
