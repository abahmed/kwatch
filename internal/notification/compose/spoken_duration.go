package compose

import "time"

// spokenSeconds says a measured span in words, the way every other
// duration of a note reads: "ten seconds", "two minutes", "one hour". A
// value that is not a duration (a detector's own words) is returned as
// it is.
func spokenSeconds(value string) string {
	d, err := time.ParseDuration(value)
	if err != nil {
		return value
	}
	return spokenFor(d)
}

// spokenFor is spokenSeconds for a duration: seconds are counted below
// a minute, longer spans use humanDuration.
func spokenFor(d time.Duration) string {
	secs := int(d.Round(time.Second).Seconds())
	switch {
	case secs == 0:
		return "less than a second"
	case d < time.Minute:
		return plural(secs, "second")
	}
	return humanDuration(d)
}
