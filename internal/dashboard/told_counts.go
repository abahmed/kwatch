package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// Untold openings. An incident that settled, recovered or moved before
// alerting reached nobody: no page and no notification went out. The
// charts count such an opening as untold, never as a page or a
// notification, so "pages this week" matches what people received.

// openingTold reports whether the opening of p at opened reached
// anyone: the current one when a message of the incident did, an
// earlier one when its occurrence was heard. An opening kwatch keeps
// no occurrence for (a blip folded into a recurrence) was not.
func openingTold(p incident.Incident, opened time.Time) bool {
	if !opened.Before(p.Opened) {
		return !p.Untold()
	}
	for _, o := range p.History {
		if o.Opened.Equal(opened) {
			return o.Heard
		}
	}
	return false
}

// addOpening counts the opening of p at opened on d: at p's tier when
// it was told, else as untold.
func (d *TrendDay) addOpening(p incident.Incident, opened time.Time) {
	if !openingTold(p, opened) {
		d.Untold++
		d.Total++
		return
	}
	d.add(p.Tier)
}

// toldOnly is the durations of the occurrences someone was alerted
// about.
func toldOnly(ds []trendDuration) []trendDuration {
	var out []trendDuration
	for _, d := range ds {
		if d.told {
			out = append(out, d)
		}
	}
	return out
}

// resolvesTold are the times p resolved after someone was alerted: its
// heard earlier occurrences and p itself when it was told.
func resolvesTold(p incident.Incident) []time.Time {
	var out []time.Time
	for _, o := range p.History {
		if !o.Resolved.IsZero() && o.Heard {
			out = append(out, o.Resolved)
		}
	}
	if p.State == incident.Resolved && p.SupersededBy == "" &&
		!p.Resolved.IsZero() && !p.Untold() {
		out = append(out, p.Resolved)
	}
	return out
}
