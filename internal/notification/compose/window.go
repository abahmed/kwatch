package compose

import (
	"strings"
	"time"
)

// windowSentences say what was recorded between the last time the root
// was healthy and the first failure, when both times are known:
//
//	Between 10:01:40 (last healthy) and 10:02:10 (first failure): bob
//	changed api config at 10:01.
//
// It states what the change history holds and does not blame anyone.
// A window that crosses midnight names the day of both ends.
func windowSentences(f caseFacts) []sentence {
	w := f.window
	if w == nil || len(w.Changes) == 0 || w.Healthy.IsZero() ||
		w.Failed.IsZero() || !w.Healthy.Before(w.Failed) {
		return nil
	}
	items := make([]string, 0, len(w.Changes))
	for _, change := range w.Changes {
		items = append(items, changeFact(change))
	}
	return []sentence{{part: partChanges, weight: 1,
		text: "Between " + windowTime(w.Healthy, w.Failed) +
			" (last healthy) and " + windowTime(w.Failed, w.Healthy) +
			" (first failure): " + strings.Join(items, "; ") + "."}}
}

// windowTime is t with seconds, and with its day when other falls on a
// different one.
func windowTime(t, other time.Time) string {
	t, other = t.UTC(), other.UTC()
	if t.YearDay() == other.YearDay() && t.Year() == other.Year() {
		return t.Format("15:04:05")
	}
	return t.Format("Jan 2 15:04:05")
}
