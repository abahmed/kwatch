package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// InsightDay is one calendar day of the last week (YYYY-MM-DD in the
// requested zone): the issues that opened, those that resolved, the
// openings that paged and those no message reached (untold: settled,
// recovered or moved before alerting, counted as /api/trends counts
// them, told_counts.go). Untold openings are in Opened too.
type InsightDay struct {
	Date     string `json:"date"`
	Opened   int    `json:"opened"`
	Resolved int    `json:"resolved"`
	Paged    int    `json:"paged"`
	Untold   int    `json:"untold"`
	// ResolvedAlerted counts the resolves of issues someone was
	// alerted about: Resolved without those that settled on their own.
	ResolvedAlerted int `json:"resolvedAlerted"`
}

// insightDays counts the last trendDays days in zone, today last, as
// /api/trends and the today card count them: every opening kwatch
// kept (reopens included), every resolve (earlier occurrences
// included, a superseded incident not), quiet issues left out. An
// earlier occurrence paged when it was urgent; the current one pages
// when the issue is urgent now.
func insightDays(
	items []incident.Incident, now time.Time, zone *time.Location,
) []InsightDay {
	w := newTrendWeek(now, zone)
	out := make([]InsightDay, trendDays)
	for i := range out {
		out[i].Date = w.start.AddDate(0, 0, i).Format(time.DateOnly)
	}
	at := func(t time.Time) *InsightDay {
		if t.After(now) {
			return nil
		}
		if i := w.day(t); i >= 0 {
			return &out[i]
		}
		return nil
	}
	for _, p := range items {
		if p.Tier == incident.Silent {
			continue
		}
		for _, opened := range openings(p) {
			if day := at(opened); day != nil {
				day.addOpening(p, opened)
			}
		}
		for _, resolved := range resolves(p) {
			if day := at(resolved); day != nil {
				day.Resolved++
			}
		}
		for _, resolved := range resolvesTold(p) {
			if day := at(resolved); day != nil {
				day.ResolvedAlerted++
			}
		}
	}
	return out
}

// addOpening counts the opening of p at opened on d: as opened, and
// as paged or untold when it was.
func (d *InsightDay) addOpening(p incident.Incident, opened time.Time) {
	d.Opened++
	if paged(p, opened) {
		d.Paged++
	}
	if !openingTold(p, opened) {
		d.Untold++
	}
}

// paged reports whether the opening of p at opened paged: an earlier
// occurrence that opened then and paged, or the current one while p
// is urgent.
func paged(p incident.Incident, opened time.Time) bool {
	for _, o := range p.History {
		if o.Opened.Equal(opened) {
			return o.Page
		}
	}
	return !opened.Before(p.Opened) && p.Tier == incident.Page &&
		!p.Untold()
}

// resolves are the times p resolved: its earlier occurrences and p
// itself when it is resolved and was not superseded.
func resolves(p incident.Incident) []time.Time {
	var out []time.Time
	for _, o := range p.History {
		if !o.Resolved.IsZero() {
			out = append(out, o.Resolved)
		}
	}
	if p.State == incident.Resolved && p.SupersededBy == "" &&
		!p.Resolved.IsZero() {
		out = append(out, p.Resolved)
	}
	return out
}
