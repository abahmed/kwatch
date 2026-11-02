package dashboard

import (
	"net/url"
	"strconv"
	"time"
)

// maxZoneName bounds ?tzName=: the longest IANA zone names are about 30
// characters.
const maxZoneName = 64

// parseZone reads the zone days are counted in: tzName, an IANA zone
// name such as "Africa/Cairo", so a week across a daylight saving change
// counts its days right; else, or when kwatch does not know that name,
// tz, whole minutes east of UTC within maxTZOffset; else UTC. A tzName
// that cannot be a zone name is an error.
func parseZone(q url.Values) (*time.Location, error) {
	if name := q.Get("tzName"); name != "" {
		if !validZoneName(name) {
			return nil, badQuery("tzName", name,
				"Not a time zone name: %q.", name)
		}
		if zone, err := time.LoadLocation(name); err == nil &&
			name != "Local" {
			return zone, nil
		}
	}
	raw := q.Get("tz")
	if raw == "" {
		return time.UTC, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < -maxTZOffset || n > maxTZOffset {
		return nil, badQuery("tz", raw, "The time zone offset must be "+
			"whole minutes from -%d to %d, not %q.", maxTZOffset,
			maxTZOffset, raw)
	}
	return offsetZone(n), nil
}

// offsetZone is the fixed zone minutes east of UTC.
func offsetZone(minutes int) *time.Location {
	if minutes == 0 {
		return time.UTC
	}
	return time.FixedZone("", minutes*60)
}

// validZoneName reports whether name can be an IANA zone name: letters,
// digits, "/", "_", "-" and "+", not starting with "/". A dot is never
// in one, so no name reaches outside the zone database.
func validZoneName(name string) bool {
	if len(name) > maxZoneName || name[0] == '/' {
		return false
	}
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '/' || r == '_' || r == '-' ||
			r == '+'
		if !ok {
			return false
		}
	}
	return true
}
