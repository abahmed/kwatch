package dashboard

import (
	"net/url"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
)

// The Issues page splits the active list three ways (?bucket=): what
// needs attention, what has gone on unchanged for over a day, and
// cleanup: leftovers nobody uses that fail without hurting anyone.
const (
	bucketAttention = "attention"
	bucketOngoing   = "ongoing"
	bucketCleanup   = "cleanup"
)

var knownBuckets = []string{bucketAttention, bucketOngoing, bucketCleanup}

// ongoingAfter is how long an issue goes on unchanged before it is
// ongoing rather than one that needs attention.
const ongoingAfter = 24 * time.Hour

// cleanupReasons are the failures that are leftovers by nature: an
// object stuck while it is deleted, a Service or claim nothing uses, an
// expired certificate.
var cleanupReasons = map[string]bool{
	reasons.StuckDeleting:  true,
	reasons.ServiceUnused:  true,
	reasons.ClaimUnused:    true,
	reasons.TLSCertExpired: true,
}

// parseBucket reads ?bucket=: one of knownBuckets, or "" for every
// active issue.
func parseBucket(q url.Values) (string, error) {
	v := q["bucket"]
	if len(v) == 0 || (len(v) == 1 && v[0] == "") {
		return "", nil
	}
	if len(v) > 1 {
		return "", badQuery("bucket", v[1],
			"Give bucket once: attention, ongoing or cleanup.")
	}
	for _, b := range knownBuckets {
		if v[0] == b {
			return b, nil
		}
	}
	return "", badQuery("bucket", v[0],
		"Unknown bucket: %q. Use attention, ongoing or cleanup.", v[0])
}

// bucketOf is the bucket of active incident p at now; "" for a
// resolved one.
func bucketOf(p incident.Incident, item IncidentListItem,
	now time.Time) string {
	switch {
	case item.State == stateResolved:
		return ""
	case p.Tier != incident.Page && isCleanup(p):
		return bucketCleanup
	case isOngoing(p, item, now):
		return bucketOngoing
	}
	return bucketAttention
}

// isCleanup reports a leftover: a group of objects a removed controller
// left behind, or an issue whose every failure is a cleanup reason and
// none critical.
func isCleanup(p incident.Incident) bool {
	if p.LeftoverObjects() > 0 {
		return true
	}
	found := false
	for _, f := range p.Members {
		if f.Advisory {
			continue
		}
		cleanup := cleanupReasons[f.Reason] ||
			f.Mode == detection.ModeStuckDeleting
		if !cleanup || f.Severity >= detection.Critical {
			return false
		}
		found = true
	}
	return found
}

// isOngoing reports an issue open for over ongoingAfter that has not
// changed in that time: it did not recover, come back or flap.
func isOngoing(p incident.Incident, item IncidentListItem,
	now time.Time) bool {
	from := now.Add(-ongoingAfter)
	if item.FirstSeen.IsZero() || item.FirstSeen.After(from) ||
		item.State == "flapping" || item.State == stateSettling {
		return false
	}
	for _, list := range [][]time.Time{p.Cycles, p.Occurrences} {
		for _, t := range list {
			if t.After(from) {
				return false
			}
		}
	}
	return p.RecoveringSince.IsZero() || !p.RecoveringSince.After(from)
}
