package incident

import (
	"time"
)

// Known problems and rhythms. A problem people have heard about for a
// day is known: its recurrences wait for the digest instead of
// interrupting again, and an incident open for a week is reminded of
// once a week. A problem that fails on a regular rhythm is known too,
// once the rhythm shows.
const (
	// KnownAfter is how long a problem must have been heard about, from
	// its first heard occurrence, before its recurrences are known.
	KnownAfter = 24 * time.Hour
	// KnownOccurrences is how many heard occurrences make a problem
	// known, with KnownAfter.
	KnownOccurrences = 2
	// RemindEvery is how often an incident that stays open is said
	// again: "still failing, for a week now".
	RemindEvery = 7 * 24 * time.Hour
	// RhythmOccurrences is the fewest occurrences within RhythmWindow
	// that can show a rhythm.
	RhythmOccurrences = 3
	// RhythmWindow is how far back a rhythm is looked for.
	RhythmWindow = 24 * time.Hour
	// RhythmTolerance is how far each interval may stray from the mean
	// interval, as a share of it, and still be the same rhythm.
	RhythmTolerance = 0.35
)

// ReasonReminder is the audit reason of a weekly "still open" update.
const ReasonReminder = "still open"

// Known reports a recurrence of a problem people have already heard
// about for KnownAfter: at least KnownOccurrences heard occurrences of
// the same failure mode, the first of them KnownAfter or more before
// now. The incident is not news; it waits for the digest. A page stays
// a page: a chronic outage is still an outage.
func Known(p Incident, now time.Time) bool {
	heard := 0
	var first time.Time
	for _, o := range p.History {
		// Why: a root known for one failure must not hide a new one.
		// An unknown mode on either side is not a match.
		if !o.Heard || p.Mode == "" || o.Mode != p.Mode {
			continue
		}
		heard++
		if first.IsZero() || o.Opened.Before(first) {
			first = o.Opened
		}
	}
	return heard >= KnownOccurrences && now.Sub(first) >= KnownAfter
}

// Rhythm reports whether the incident's occurrences in the last
// RhythmWindow come at regular intervals, and the mean interval. A
// controller that restarts every forty minutes shows one; a problem
// that comes and goes at random does not.
func Rhythm(p Incident, now time.Time) (time.Duration, bool) {
	var recent []time.Time
	for _, at := range p.Occurrences {
		if now.Sub(at) <= RhythmWindow {
			recent = append(recent, at)
		}
	}
	if len(recent) < RhythmOccurrences {
		return 0, false
	}
	mean := recent[len(recent)-1].Sub(recent[0]) /
		time.Duration(len(recent)-1)
	if mean <= 0 {
		return 0, false
	}
	for i := 1; i < len(recent); i++ {
		gap := recent[i].Sub(recent[i-1])
		if deviation(gap, mean) > RhythmTolerance {
			return 0, false
		}
	}
	return mean, true
}

// deviation is how far gap strays from mean, as a share of mean.
func deviation(gap, mean time.Duration) float64 {
	diff := float64(gap - mean)
	if diff < 0 {
		diff = -diff
	}
	return diff / float64(mean)
}
