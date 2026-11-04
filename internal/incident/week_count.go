package incident

import "time"

// weekCount is how many times the root failed within recurrenceWeek of
// now, this time included: the earlier occurrences History keeps. It is
// the one count the timeline note and the dashboard share; Occurrences
// also hold the openings of failures that moved to another incident,
// which are no time of their own.
func weekCount(history []Occurrence, now time.Time) int {
	n := 1
	for _, o := range history {
		if !o.Opened.IsZero() && now.Sub(o.Opened) <= recurrenceWeek {
			n++
		}
	}
	return n
}

// WeekCount is weekCount of p's history: how many times its root
// failed this week, this time included.
func (p Incident) WeekCount(now time.Time) int {
	return weekCount(p.History, now)
}

// lastTimeAt names when the previous occurrence opened, in UTC.
func lastTimeAt(previous *Incident) string {
	return previous.Opened.UTC().Format("Mon 15:04") + " UTC"
}
