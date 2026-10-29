package dashboard

import (
	"cmp"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// maxQueryLength bounds the search text.
const maxQueryLength = 200

// List states: an incident is active until it resolves.
const (
	stateActive   = "active"
	stateResolved = "resolved"
)

var (
	knownSeverities = []string{"page", "notify", "digest", "silent"}
	// activeStates narrow the active list.
	activeStates = []string{"settling", "open", "recovering", "flapping"}
)

// filter is the /api/incidents query. state picks the list; the rest
// narrow it. Every repeatable value is OR-ed, different keys are AND-ed.
type filter struct {
	state      string
	namespace  string
	text       string
	severities map[string]bool
	states     map[string]bool
	since      time.Time
}

// parseFilter reads the query. state is repeatable: active or resolved
// picks the list, and settling, open, recovering and flapping narrow the
// active list. since is a Go duration back from now ("24h") or an RFC
// 3339 time.
func parseFilter(q url.Values, now time.Time) (filter, error) {
	f := filter{namespace: q.Get("namespace"),
		text: strings.ToLower(strings.TrimSpace(q.Get("q")))}
	if len(f.text) > maxQueryLength {
		return f, errors.New("invalid q")
	}
	var err error
	if f.state, f.states, err = parseStates(q["state"]); err != nil {
		return f, err
	}
	if f.severities, err = parseSet(q["severity"],
		knownSeverities); err != nil {
		return f, errors.New("invalid severity")
	}
	if f.since, err = parseSince(q.Get("since"), now); err != nil {
		return f, errors.New("invalid since")
	}
	return f, nil
}

func parseStates(values []string) (string, map[string]bool, error) {
	list := ""
	var narrow []string
	for _, v := range splitValues(values) {
		switch {
		case v == stateActive || v == stateResolved:
			if list != "" && list != v {
				return "", nil, errors.New("invalid state")
			}
			list = v
		case slices.Contains(activeStates, v):
			narrow = append(narrow, v)
		default:
			return "", nil, errors.New("invalid state")
		}
	}
	if len(narrow) == 0 {
		return list, nil, nil
	}
	if list == stateResolved {
		return "", nil, errors.New("invalid state")
	}
	set, _ := parseSet(narrow, activeStates)
	return stateActive, set, nil
}

// parseSet reads repeatable values, comma-separated or repeated, each
// one of known; nil when there are none.
func parseSet(values, known []string) (map[string]bool, error) {
	var out map[string]bool
	for _, v := range splitValues(values) {
		if !slices.Contains(known, v) {
			return nil, errors.New("unknown value")
		}
		if out == nil {
			out = map[string]bool{}
		}
		out[v] = true
	}
	return out, nil
}

func splitValues(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func parseSince(raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return now.Add(-d), nil
	}
	return time.Parse(time.RFC3339, raw)
}

// inList reports whether p belongs to the requested list. The active
// list leaves out silent incidents unless the silent severity is asked
// for (decision D4): kwatch tracks them but sends nothing.
func (f filter) inList(p incident.Incident) bool {
	resolved := p.State == incident.Resolved
	switch f.state {
	case stateActive:
		if resolved {
			return false
		}
		return p.Tier != incident.Silent || f.severities["silent"]
	case stateResolved:
		return resolved
	}
	return true
}

// matches applies the narrowing filters to an item of the list.
func (f filter) matches(p incident.Incident, item IncidentListItem) bool {
	if f.namespace != "" && item.Namespace != f.namespace {
		return false
	}
	if f.severities != nil && !f.severities[item.Severity] {
		return false
	}
	if f.states != nil && !f.states[item.State] {
		return false
	}
	if !f.since.IsZero() && sinceTime(item).Before(f.since) {
		return false
	}
	return f.text == "" || searchable(p, item).contains(f.text)
}

// sinceTime is what since compares: when a resolved incident resolved,
// when an active one was last seen.
func sinceTime(item IncidentListItem) time.Time {
	if !item.ResolvedAt.IsZero() {
		return item.ResolvedAt
	}
	return item.LastSeen
}

type haystack []string

func (h haystack) contains(needle string) bool {
	for _, s := range h {
		if strings.Contains(strings.ToLower(s), needle) {
			return true
		}
	}
	return false
}

// searchable is what q looks in: the title, where it is, the ID and the
// cause sentence.
func searchable(p incident.Incident, item IncidentListItem) haystack {
	// The cause is searched as shown, redacted: matching the raw text
	// would let a search tell what a redacted secret contains.
	return haystack{item.Title, item.Namespace, item.Workload, item.ID,
		item.Namespace + " / " + item.Workload,
		redact.Evidence(causeText(p))}
}

// order sorts a list: urgent first, then newest, on the active list
// (decision D4); latest resolved first on the resolved list (D5);
// newest first otherwise.
func (f filter) order(a, b IncidentListItem) int {
	switch f.state {
	case stateActive:
		if c := cmp.Compare(urgency(b), urgency(a)); c != 0 {
			return c
		}
	case stateResolved:
		if c := b.ResolvedAt.Compare(a.ResolvedAt); c != 0 {
			return c
		}
	}
	return newestFirst(a, b)
}

func urgency(item IncidentListItem) int {
	if item.Severity == "page" {
		return 1
	}
	return 0
}

// newestFirst orders by first seen, newest first, then by ID.
func newestFirst(a, b IncidentListItem) int {
	if c := b.FirstSeen.Compare(a.FirstSeen); c != 0 {
		return c
	}
	return cmp.Compare(b.ID, a.ID)
}
