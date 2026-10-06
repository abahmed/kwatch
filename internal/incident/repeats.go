package incident

import (
	"strconv"
	"time"
)

// repeatsPage reports a page-tier incident that re-opened a page which
// resolved within RepageWindow, and how many times it came back in that
// window, this one included.
func repeatsPage(p *Incident, now time.Time) (int, bool) {
	count := 0
	for _, o := range p.History {
		if o.Page && now.Sub(o.Resolved) <= RepageWindow &&
			(p.Mode == "" || o.Mode == "" || o.Mode == p.Mode) {
			count++
		}
	}
	return count + 1, count > 0
}

// heardTimes counts how many times the incident was told about within
// RepageWindow of now, this return included.
func heardTimes(p *Incident, now time.Time) int {
	count := 1
	for _, o := range p.History {
		if o.Heard && now.Sub(o.Resolved) <= RepageWindow &&
			(p.Mode == "" || o.Mode == "" || o.Mode == p.Mode) {
			count++
		}
	}
	return count
}

// RepeatPhrase words how often an incident came back within
// RepageWindow: "4th time in 2h". Writers humanize it with the count
// they read from Incident.RepeatCount.
func RepeatPhrase(times int) string {
	return ordinal(times) + " time in " + windowText(RepageWindow)
}

// repeatNote words a held repeat in the timeline: "failing again: 4th
// time in 2h".
func repeatNote(times int) string {
	return "failing again: " + RepeatPhrase(times)
}

// windowText words a duration as whole hours or whole minutes: "2h",
// "90m". Duration.String would give "2h0m0s".
func windowText(d time.Duration) string {
	if d%time.Hour == 0 {
		return strconv.Itoa(int(d/time.Hour)) + "h"
	}
	return strconv.Itoa(int(d/time.Minute)) + "m"
}

// DigestWorthy reports whether a digest-tier incident that resolved
// before its digest went out is still worth a line in it. A first
// occurrence is a blip nobody needs. A recurrence is reported, so a
// problem that comes back after each resolve does not stay unseen, except
// that periodic noise (a rhythm) is reported only when no digest listed
// it in the last DigestReportEvery.
func DigestWorthy(p Incident, now time.Time) bool {
	if len(p.History) == 0 {
		return false
	}
	if !hasRhythm(&p) {
		return true
	}
	last := p.DigestedAt
	for _, o := range p.History {
		if o.Digested.After(last) {
			last = o.Digested
		}
	}
	return last.IsZero() || now.Sub(last) >= DigestReportEvery
}
