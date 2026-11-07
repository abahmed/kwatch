package format

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Day and Week are the long spans shared by timings and wording.
const (
	Day  = 24 * time.Hour
	Week = 7 * Day
)

// OrDefault returns value when it is non-empty, otherwise fallback.
func OrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// JoinNames lists up to max names, sorted, and summarises the rest as
// "+N more". Names are deduplicated first.
func JoinNames(names []string, max int) string {
	seen := make(map[string]bool, len(names))
	uniq := make([]string, 0, len(names))
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		uniq = append(uniq, n)
	}
	sort.Strings(uniq)
	if max <= 0 || len(uniq) <= max {
		return strings.Join(uniq, ", ")
	}
	return fmt.Sprintf(
		"%s +%d more",
		strings.Join(uniq[:max], ", "),
		len(uniq)-max,
	)
}

// Duration renders d for people: "5s", "2m30s", "2h15m". Sub-second detail is
// dropped — an alert about a pod that has been stuck for 12 minutes does not
// need milliseconds.
func Duration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	// A trailing zero unit is noise: "1m0s" is "1m", "2h0m" is "2h".
	if d < time.Hour {
		if s := int(d.Seconds()) % 60; s != 0 {
			return fmt.Sprintf("%dm%ds", int(d.Minutes()), s)
		}
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if m := int(d.Minutes()) % 60; m != 0 {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), m)
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
