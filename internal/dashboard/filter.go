package dashboard

import (
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/redact"
)

// maxQueryLength bounds the search text, maxSearchTerms the number of
// q and exclude terms together (each one is looked for in every
// incident), maxLikeLength the incident ID of ?like=.
const (
	maxQueryLength = 200
	maxSearchTerms = 10
	maxLikeLength  = 128
)

// List states: an incident is active until it resolves.
const (
	stateActive   = "active"
	stateResolved = "resolved"
)

var (
	knownSeverities = []string{"page", "notify", "digest", "silent"}
	// activeStates are the states of an active issue.
	activeStates = []string{"settling", "open", "recovering", "flapping"}
	knownStates  = append(slices.Clone(activeStates), stateResolved)
	ackedValues  = []string{"true", "false"}
)

// filter is the /api/incidents query. status picks the list; the rest
// narrow it. Every repeatable value is OR-ed, different keys are AND-ed,
// except q, where every value must match, and exclude, where none may.
type filter struct {
	// status is active, resolved or "" for both.
	status     string
	namespaces map[string]bool
	// kinds are lower-case root kinds: deployment, node.
	kinds map[string]bool
	// texts and excludes are lower-case search terms.
	texts      []string
	excludes   []string
	severities map[string]bool
	states     map[string]bool
	since      time.Time
	// startedAfter and startedBefore bound when an incident was first
	// seen, for "new today" and "going on over a day".
	startedAfter  time.Time
	startedBefore time.Time
	// red redacts text the search reads; nil is redact.Evidence.
	red func(string) string
	// acked narrows to acknowledged ("true") or not ("false") issues;
	// both, like none, keep every issue.
	acked map[string]bool
	// like is the incident ID of ?like=: only incidents with the same
	// error (similar.go). likeKey is that incident's key, set by
	// buildList; "" matches nothing.
	like    string
	likeKey string
	// nodes are node names (?node=): only incidents whose pods run on
	// one of them, or that are about one. onNodes is the IDs of those
	// incidents, set by the handler from the inventory.
	nodes   map[string]bool
	onNodes map[string]bool
	// modes are failure families (?mode=, insights_causes.go).
	modes map[string]bool
}

// parseFilter reads the query. status is active (the default), resolved
// or all. severity, state, namespace, kind (the root's kind, any case)
// and acked (true, false) are repeatable: the values of one are OR-ed.
// since, startedAfter and startedBefore are Go durations back from now
// ("24h"). q and exclude are repeatable search terms, at most
// maxSearchTerms of them and maxQueryLength characters together. like
// is one incident ID.
func parseFilter(q url.Values, now time.Time) (filter, error) {
	f := filter{namespaces: anySet(q["namespace"]),
		kinds:    anySet(lowerTerms(q["kind"])),
		texts:    lowerTerms(q["q"]),
		excludes: lowerTerms(q["exclude"])}
	if len(f.texts)+len(f.excludes) > maxSearchTerms {
		return f, errors.New("too many search words")
	}
	if termsLength(f.texts)+termsLength(f.excludes) > maxQueryLength {
		return f, errors.New("invalid q")
	}
	if len(q["like"]) > 1 || len(q.Get("like")) > maxLikeLength {
		return f, errors.New("invalid like")
	}
	f.like = q.Get("like")
	var err error
	if f.nodes, f.modes, err = parseNodesModes(q); err != nil {
		return f, err
	}
	if f.status, err = parseStatus(q.Get("status")); err != nil {
		return f, err
	}
	if f.states, err = parseSet(q["state"], knownStates); err != nil {
		return f, errors.New("invalid state")
	}
	if f.acked, err = parseSet(q["acked"], ackedValues); err != nil {
		return f, errors.New("invalid acked")
	}
	if f.severities, err = parseSet(q["severity"],
		knownSeverities); err != nil {
		return f, errors.New("invalid severity")
	}
	return parseTimes(q, now, f)
}

// maxNodeName bounds a ?node= value: a Kubernetes object name.
const maxNodeName = 253

// parseNodesModes reads the repeatable node and mode filters.
func parseNodesModes(q url.Values) (nodes, modes map[string]bool,
	err error) {
	for _, n := range q["node"] {
		if n == "" || len(n) > maxNodeName {
			return nil, nil, errors.New("invalid node")
		}
	}
	for _, m := range q["mode"] {
		if !validMode(m) {
			return nil, nil, errors.New("invalid mode")
		}
	}
	return anySet(q["node"]), anySet(q["mode"]), nil
}

// parseStatus reads status: active (the default), resolved or all.
func parseStatus(raw string) (string, error) {
	switch raw {
	case "", stateActive:
		return stateActive, nil
	case stateResolved:
		return raw, nil
	case "all":
		return "", nil
	}
	return "", errors.New("invalid status")
}

// parseTimes reads since, startedAfter and startedBefore.
func parseTimes(q url.Values, now time.Time, f filter) (filter, error) {
	var err error
	if f.since, err = parseAgo(q.Get("since"), now); err != nil {
		return f, errors.New("invalid since")
	}
	if f.startedAfter, err = parseAgo(q.Get("startedAfter"),
		now); err != nil {
		return f, errors.New("invalid startedAfter")
	}
	if f.startedBefore, err = parseAgo(q.Get("startedBefore"),
		now); err != nil {
		return f, errors.New("invalid startedBefore")
	}
	return f, nil
}

// parseSet reads repeated values, each one of known; nil when there
// are none.
func parseSet(values, known []string) (map[string]bool, error) {
	var out map[string]bool
	for _, v := range values {
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

// anySet is values as a set; nil when there are none.
func anySet(values []string) map[string]bool {
	var out map[string]bool
	for _, v := range values {
		if out == nil {
			out = map[string]bool{}
		}
		out[v] = true
	}
	return out
}

// lowerTerms trims and lower-cases search terms, dropping empty ones.
func lowerTerms(values []string) []string {
	var out []string
	for _, v := range values {
		if v = strings.ToLower(strings.TrimSpace(v)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func termsLength(terms []string) int {
	n := 0
	for _, t := range terms {
		n += len(t)
	}
	return n
}

// parseAgo reads a positive Go duration as that long before now.
func parseAgo(raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return time.Time{}, errors.New("invalid duration")
	}
	return now.Add(-d), nil
}

// statusOK reports whether item is in the requested list (active,
// resolved or both) and in one of the narrowed states.
func (f filter) statusOK(item IncidentListItem) bool {
	resolved := item.State == stateResolved
	if (f.status == stateActive && resolved) ||
		(f.status == stateResolved && !resolved) {
		return false
	}
	return f.states == nil || f.states[item.State]
}

// severityOK applies the severity filter. Without one, quiet (silent)
// issues that are still active are left out (decision D4): kwatch
// tracks them but sends nothing.
func (f filter) severityOK(item IncidentListItem) bool {
	if f.severities != nil {
		return f.severities[item.Severity]
	}
	return item.Severity != "silent" || item.State == stateResolved
}

func (f filter) namespaceOK(item IncidentListItem) bool {
	return f.namespaces == nil || f.namespaces[item.Namespace]
}

// kindOK applies the kind filter: the root's kind, any case.
func (f filter) kindOK(item IncidentListItem) bool {
	return f.kinds == nil || f.kinds[strings.ToLower(item.WorkloadKind)]
}

func (f filter) ackedOK(item IncidentListItem) bool {
	return f.acked == nil || f.acked[strconv.FormatBool(item.AckedBy != "")]
}

// restOK applies the filters that are not facets: time, like and the
// search. since bounds resolved issues only: an active issue is going
// on now.
func (f filter) restOK(p incident.Incident, item IncidentListItem) bool {
	if !f.since.IsZero() && item.State == stateResolved &&
		item.ResolvedAt.Before(f.since) {
		return false
	}
	if !f.startedAfter.IsZero() &&
		item.FirstSeen.Before(f.startedAfter) {
		return false
	}
	if !f.startedBefore.IsZero() &&
		!item.FirstSeen.Before(f.startedBefore) {
		return false
	}
	if f.modes != nil && !f.modes[modeFamily(p)] {
		return false
	}
	if f.nodes != nil && !f.onNodes[p.ID] {
		return false
	}
	return f.likeOK(p) && f.matchesText(p, item)
}

// matchesText applies the search: every q term is found, no exclude
// term is.
func (f filter) matchesText(p incident.Incident, item IncidentListItem) bool {
	if len(f.texts) == 0 && len(f.excludes) == 0 {
		return true
	}
	red := f.red
	if red == nil {
		red = redact.Evidence
	}
	h := searchable(p, item, red)
	for _, t := range f.texts {
		if !h.contains(t) {
			return false
		}
	}
	for _, t := range f.excludes {
		if h.contains(t) {
			return false
		}
	}
	return true
}

// haystack is the lower-cased text a search looks in.
type haystack []string

func (h haystack) contains(needle string) bool {
	for _, s := range h {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

// searchable is what q looks in: the title, where it is, the ID and the
// cause sentence, lower-cased once per incident rather than per term.
func searchable(
	p incident.Incident, item IncidentListItem, red func(string) string,
) haystack {
	// The cause is searched as shown, redacted: matching the raw text
	// would let a search tell what a redacted secret contains.
	h := haystack{item.Title, item.Namespace, item.Workload, item.ID,
		item.Namespace + " / " + item.Workload,
		red(causeText(p))}
	for i, s := range h {
		h[i] = strings.ToLower(s)
	}
	return h
}
