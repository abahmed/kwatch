package scorecard

import (
	"sort"
	"time"
)

// PeakInWindow returns the most timestamps that fall inside any window of
// length d, such as the loudest hour of a day or the loudest two minutes
// of a storm. A timestamp exactly d after another is in the next window.
func PeakInWindow(times []time.Time, d time.Duration) int {
	sorted := append([]time.Time(nil), times...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Before(sorted[j])
	})
	peak, start := 0, 0
	for end, at := range sorted {
		for !at.Before(sorted[start].Add(d)) {
			start++
		}
		peak = max(peak, end-start+1)
	}
	return peak
}

// BusiestDay returns the calendar day (UTC, as 2006-01-02) with the
// most timestamps and how many it has; the earliest such day on a tie.
func BusiestDay(times []time.Time) (day string, count int) {
	per := map[string]int{}
	for _, at := range times {
		per[at.UTC().Format(time.DateOnly)]++
	}
	for d, n := range per {
		if n > count || n == count && d < day {
			day, count = d, n
		}
	}
	return day, count
}
