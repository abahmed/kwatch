package dashboard

import (
	"hash/fnv"
	"strconv"
	"strings"
	"time"
)

// itemRev fingerprints what a list row shows: a row whose rev is the
// same needs no redraw.
func itemRev(item IncidentListItem) string {
	return shortHash(strings.Join([]string{item.ID, item.Title,
		item.Severity, item.State, item.Namespace, item.Workload,
		item.WorkloadKind, stamp(item.FirstSeen), stamp(item.LastSeen),
		stamp(item.ResolvedAt), strconv.Itoa(item.RepeatCount),
		item.AckedBy, item.RootCause,
		strconv.FormatBool(item.StoppedTracking),
		strconv.FormatBool(item.Untold),
		strconv.Itoa(item.LeftoverObjects)}, "\x00"))
}

// bodyETag is the entity tag of a response body: equal bodies, equal
// tags, so a poll of an unchanged list is answered 304.
func bodyETag(body []byte) string {
	h := fnv.New64a()
	_, _ = h.Write(body)
	return `"` + hex16(h.Sum64()) + `"`
}

// etagMatches reports whether an If-None-Match header names tag.
func etagMatches(header, tag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "W/")
		if part == tag || part == "*" {
			return true
		}
	}
	return false
}

func shortHash(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return hex16(h.Sum64())
}

func hex16(v uint64) string {
	s := strconv.FormatUint(v, 16)
	return strings.Repeat("0", 16-len(s)) + s
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return strconv.FormatInt(t.UnixNano(), 36)
}
