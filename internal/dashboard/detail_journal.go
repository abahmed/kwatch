package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// addJournal fills the parts of d that come from p's own record: its
// break snapshots, transitions, why it is open and its acknowledgement.
func addJournal(d *IncidentDetail, p incident.Incident, now time.Time) {
	for _, b := range p.Journal.Breaks {
		d.Snapshots = append(d.Snapshots, BreakSnapshot{At: b.At,
			Reason: b.Reason, Containers: toContainerRows(b.Containers)})
	}
	if n := len(d.Snapshots); n > 0 {
		latest := d.Snapshots[n-1]
		d.Snapshot = &latest
	}
	for _, t := range p.Journal.Transitions {
		d.Transitions = append(d.Transitions, Transition{At: t.At,
			Kind: t.Kind, From: t.From, To: t.To})
	}
	d.Why = whyOf(p, now)
	d.Ack = ackOf(p)
}

// whyOf serves incident.WhyOf; nil when it has nothing to say.
func whyOf(p incident.Incident, now time.Time) *WhyDetail {
	if now.IsZero() {
		now = p.Journal.Timers.At
	}
	w := incident.WhyOf(p, now)
	out := WhyDetail{AlertLevelReason: w.AlertLevel,
		StillOpenReason: w.StillOpen,
		HoldTotal:       int(w.HoldTotal.Seconds()),
		HoldElapsed:     int(w.HoldElapsed.Seconds()),
		CannotSee:       w.CannotSee, PagingHeldReason: w.PagingHeld}
	if out == (WhyDetail{}) {
		return nil
	}
	return &out
}

// ackOf is p's acknowledgement, its note redacted again; nil without.
func ackOf(p incident.Incident) *AckDetail {
	if p.Ack == nil {
		return nil
	}
	return &AckDetail{Note: redact.Evidence(p.Ack.Note),
		Since: p.Ack.Since, By: redact.Evidence(p.Ack.By),
		On: p.Ack.On.String()}
}
