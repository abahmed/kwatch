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
