package dashboard

import (
	"net/url"
	"time"
)

// parseTimes reads since, startedAfter and startedBefore.
func parseTimes(q url.Values, now time.Time, f filter) (filter, error) {
	var err error
	if f.since, err = parseAgo("since", q.Get("since"), now); err != nil {
		return f, err
	}
	if f.startedAfter, err = parseAgo("startedAfter",
		q.Get("startedAfter"), now); err != nil {
		return f, err
	}
	f.startedBefore, err = parseAgo("startedBefore",
		q.Get("startedBefore"), now)
	return f, err
}

// parseAgo reads the positive Go duration of key as that long before
// now.
func parseAgo(key, raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return time.Time{}, badQuery(key, raw,
			"Not a length of time: %q. Use a number and a unit, "+
				"like 24h.", raw)
	}
	if d <= 0 {
		return time.Time{}, badQuery(key, raw,
			"%s must be more than zero.", raw)
	}
	return now.Add(-d), nil
}
