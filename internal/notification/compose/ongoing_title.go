package compose

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// OngoingTitle is the line a digest says about a problem an earlier digest
// listed that is still open. It is the incident's own reminder wording
// ("Autoscaler istiod in istio-system is still failing, for four days
// now."), so one problem reads the same in every digest and never shows a
// reason code.
func (Writer) OngoingTitle(p incident.Incident, now time.Time) string {
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: incident.ReasonReminder}
	// Like the other digest lines, it leaves the cluster tag to the header.
	return withUsualTail(Writer{}.Write(d, now).Title, d)
}

// StartedAt is when the problem began: the earlier of the incident's
// opening and the first of its findings, as Kubernetes dated it. A
// restart or a reopen moves the opening, never the finding, so a Service
// stuck deleting since April does not read "since Oct 6".
func StartedAt(p incident.Incident) time.Time {
	first := p.Opened
	for _, m := range p.Members {
		if m.Advisory || m.Since.IsZero() {
			continue
		}
		if first.IsZero() || m.Since.Before(first) {
			first = m.Since
		}
	}
	return first
}
