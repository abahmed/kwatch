package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// seenWindowDays is how far back "seen before" counts occurrences.
const seenWindowDays = 30

// phaseWindow bounds the states strip to the latest day of a long
// incident.
const phaseWindow = 24 * time.Hour

// Phase states.
const (
	phaseFailing    = "failing"
	phaseRecovering = "recovering"
	phaseQuiet      = "quiet"
)

// seenBefore summarises the earlier occurrences kwatch remembers of
// p's root, newest first; nil when there are none.
func seenBefore(
	p incident.Incident, now time.Time, known func(string) bool,
) *SeenBefore {
	if len(p.History) == 0 {
		return nil
	}
	ref := now
	if ref.IsZero() {
		ref = lastSeen(p)
	}
	since := ref.AddDate(0, 0, -seenWindowDays)
	out := &SeenBefore{WindowDays: seenWindowDays,
		Capped: len(p.History) >= incident.HistoryKept,
		Recent: make([]PastOccurrence, 0, len(p.History))}
	for i := len(p.History) - 1; i >= 0; i-- {
		o := p.History[i]
		if !o.Opened.Before(since) {
			out.InWindow++
		}
		out.Recent = append(out.Recent, PastOccurrence{
			Opened: o.Opened, Resolved: o.Resolved, Fix: string(o.Fix),
			Mode: redact.Evidence(string(o.Mode))})
	}
	if p.Previous != "" && known != nil && known(p.Previous) {
		out.PreviousID = p.Previous
	}
	return out
}

// phasesOf is p's states over its lifetime, or the last phaseWindow of
// it: earlier occurrences of the same failure as failing, the time
// between them as quiet, and the current one as failing, then
// recovering when it looks better. Nil without an open time.
func phasesOf(p incident.Incident, now time.Time) []Phase {
	end := p.Resolved
	if end.IsZero() {
		end = now
	}
	if end.IsZero() {
		end = lastSeen(p)
	}
	if p.Opened.IsZero() || end.Before(p.Opened) {
		return nil
	}
	start := end.Add(-phaseWindow)
	var out []Phase
	var last time.Time
	for _, o := range p.History {
		if o.Resolved.IsZero() || !o.Resolved.After(start) ||
			!o.Opened.Before(p.Opened) || o.Resolved.Before(last) {
			continue
		}
		out = appendQuiet(out, last, later(o.Opened, start, last))
		out = append(out, Phase{State: phaseFailing,
			From: later(o.Opened, start, last), To: o.Resolved})
		last = o.Resolved
	}
	opened := later(p.Opened, start, last)
	out = appendQuiet(out, last, opened)
	return append(out, currentPhases(p, opened)...)
}

// currentPhases are the phases of the current occurrence from opened.
// An open incident's last phase has no end: it lasts until now.
func currentPhases(p incident.Incident, opened time.Time) []Phase {
	back := p.RecoveringSince
	if p.Resolved.IsZero() && back.After(opened) {
		return []Phase{
			{State: phaseFailing, From: opened, To: back},
			{State: phaseRecovering, From: back},
		}
	}
	return []Phase{{State: phaseFailing, From: opened, To: p.Resolved}}
}

// appendQuiet adds the quiet time from to to, when there is some.
func appendQuiet(out []Phase, from, to time.Time) []Phase {
	if from.IsZero() || !to.After(from) {
		return out
	}
	return append(out, Phase{State: phaseQuiet, From: from, To: to})
}

// later is the latest of the times given.
func later(first time.Time, rest ...time.Time) time.Time {
	out := first
	for _, t := range rest {
		if t.After(out) {
			out = t
		}
	}
	return out
}
