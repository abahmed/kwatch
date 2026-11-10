package dashboard

import (
	"slices"

	"github.com/abahmed/kwatch/internal/incident"
)

// maxSimilarScan bounds how many incidents one similar count or like
// filter reads: far more than kwatch keeps in a week.
const maxSimilarScan = 20_000

// similarKey is what "the same error" means: the key kwatch stored on
// the incident when it was announced (its failure mode and the rule of
// its likely cause; see incident.SameErrorKey), or the key of its mode
// and cause now for one not announced yet or saved by an older kwatch.
// An incident with neither has no key and is similar to nothing.
func similarKey(p incident.Incident) string {
	return p.ErrorKey()
}

// likeKey is the similar key of the incident id among incidents, or
// "" when it is not there: an expired link then matches nothing.
func likeKey(id string, incidents []incident.Incident) string {
	for i, p := range incidents {
		if i >= maxSimilarScan {
			break
		}
		if p.ID == id {
			return similarKey(p)
		}
	}
	return ""
}

// likeOK applies ?like=: an incident with the same key, not the one
// named itself.
func (f filter) likeOK(p incident.Incident) bool {
	if f.like == "" {
		return true
	}
	return f.likeKey != "" && p.ID != f.like &&
		similarKey(p) == f.likeKey
}

// countSimilar is how many other incidents share p's key and would be
// listed by /api/incidents?status=all&like=<p.ID>: quiet issues that
// are still active are left out there, as on every list without a
// severity filter.
func countSimilar(p incident.Incident, all []incident.Incident) int {
	key := similarKey(p)
	if key == "" {
		return 0
	}
	n := 0
	for i, q := range all {
		if i >= maxSimilarScan {
			break
		}
		if q.ID == p.ID || similarKey(q) != key {
			continue
		}
		if q.Tier == incident.Silent && q.State != incident.Resolved {
			continue
		}
		n++
	}
	return n
}

// maxSimilarRefs bounds how many similar issues the detail names.
const maxSimilarRefs = 3

// SimilarRef names one other issue with the same error.
type SimilarRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	State string `json:"state"`
}

// similarRefs names up to maxSimilarRefs of the incidents countSimilar
// counts: the ones still going first, then the newest.
func similarRefs(p incident.Incident, all []incident.Incident) []SimilarRef {
	key := similarKey(p)
	if key == "" {
		return nil
	}
	var found []incident.Incident
	for i, q := range all {
		if i >= maxSimilarScan {
			break
		}
		if q.ID == p.ID || similarKey(q) != key ||
			(q.Tier == incident.Silent && q.State != incident.Resolved) {
			continue
		}
		found = append(found, q)
	}
	slices.SortStableFunc(found, func(a, b incident.Incident) int {
		ra, rb := a.State == incident.Resolved, b.State == incident.Resolved
		if ra != rb && ra {
			return 1
		} else if ra != rb {
			return -1
		}
		return b.Opened.Compare(a.Opened)
	})
	if len(found) > maxSimilarRefs {
		found = found[:maxSimilarRefs]
	}
	out := make([]SimilarRef, 0, len(found))
	for _, q := range found {
		out = append(out, SimilarRef{ID: q.ID, Title: toListItem(q).Title,
			State: q.State.String()})
	}
	return out
}
