package dashboard

import (
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// maxListTitle bounds the title a list item carries, like maxListCause:
// one line on a row. The detail carries it whole.
const maxListTitle = 200

// maxListItemBytes bounds one list item as JSON, so a full page of 100
// items, the facets and the envelope always fit maxResponseBytes: 100
// items of 591 bytes, facets of at most about 5.5 KB.
const maxListItemBytes = 590

// Facet bounds: the namespaces and kinds with the most issues.
const (
	maxNamespaceFacets = 50
	maxKindFacets      = 20
)

// The facet filters: a row passes each or not, and each facet counts
// the rows passing every other one.
const (
	passStatus = 1 << iota
	passSeverity
	passNamespace
	passKind
	passAcked
	passStopped
	passAll = passStatus | passSeverity | passNamespace | passKind |
		passAcked | passStopped
)

// listRowOf is an incident as a list row with the facet filters it
// passes (pass* bits).
type listRowOf struct {
	item IncidentListItem
	pass int
}

// buildList answers a list request over every incident: facets over
// the filtered set, then one page of it.
func buildList(req listRequest, incidents []incident.Incident,
	now time.Time, red func(string) string) ListResponse {
	f := req.filter
	f.red = red
	if f.like != "" {
		f.likeKey = likeKey(f.like, incidents)
	}
	rows := make([]listRowOf, 0, len(incidents))
	for _, p := range incidents {
		item := listItem(p, red)
		if !f.restOK(p, item) {
			continue
		}
		rows = append(rows, listRowOf{item: item, pass: f.passes(item)})
	}
	out := ListResponse{Page: req.page, PageSize: req.pageSize,
		Facets: newFacets(), ModeLabels: modeLabelsOf(f.modes)}
	matched := addFacets(&out.Facets, rows)
	slices.SortFunc(matched, listOrder(req.sort, req.desc, f.status, now))
	out.Total = len(matched)
	out.TotalPages = (out.Total + req.pageSize - 1) / req.pageSize
	out.Incidents = pageOf(matched, req.page, req.pageSize)
	return out
}

// pageOf is page (1-based) of items, each fitted to maxListItemBytes;
// empty past the last page.
func pageOf(items []IncidentListItem, page, size int) []IncidentListItem {
	out := make([]IncidentListItem, 0)
	for i := (page - 1) * size; i < len(items) && len(out) < size; i++ {
		out = append(out, fitItem(items[i]))
	}
	return out
}

// fitItem shortens the free text of item (cause, title, object name,
// acknowledgement) by halves until its JSON fits maxListItemBytes.
func fitItem(item IncidentListItem) IncidentListItem {
	for limit := maxListTitle / 2; limit >= 8 &&
		itemBytes(item) > maxListItemBytes; limit /= 2 {
		for _, s := range []*string{&item.RootCause, &item.Title,
			&item.Workload, &item.AckedBy} {
			*s, _ = shortText(*s, limit)
		}
	}
	return item
}

func itemBytes(item IncidentListItem) int {
	body, err := json.Marshal(item)
	if err != nil {
		return 0
	}
	return len(body)
}

// listItem is the list row of p: its title cut to one line and its
// fingerprint.
func listItem(
	p incident.Incident, red func(string) string,
) IncidentListItem {
	item := toListItemWith(p, red)
	item.Title, _ = shortText(item.Title, maxListTitle)
	item.Rev = itemRev(item)
	return item
}

// passes is the pass* bits of the facet filters item passes.
func (f filter) passes(item IncidentListItem) int {
	return bitIf(f.statusOK(item), passStatus) |
		bitIf(f.severityOK(item), passSeverity) |
		bitIf(f.namespaceOK(item), passNamespace) |
		bitIf(f.kindOK(item), passKind) |
		bitIf(f.ackedOK(item), passAcked) |
		bitIf(f.stoppedOK(item), passStopped)
}

func bitIf(ok bool, bit int) int {
	if ok {
		return bit
	}
	return 0
}

// passesBut reports whether a row passes every facet filter but bit.
func (r listRowOf) passesBut(bit int) bool {
	return r.pass|bit == passAll
}

func newFacets() ListFacets {
	return ListFacets{
		Severity: map[string]int{"page": 0, "notify": 0, "digest": 0,
			"silent": 0},
		State: map[string]int{"open": 0, "settling": 0, "flapping": 0,
			"recovering": 0, stateResolved: 0},
		Namespace: map[string]int{},
		Kind:      map[string]int{},
		Acked:     map[string]int{"true": 0, "false": 0},
		Stopped:   map[string]int{"true": 0, "false": 0},
	}
}

// addFacets counts each facet over the rows passing every other filter
// and returns the rows passing all of them. The state facet ignores
// status too, so the status switch can show each list's size.
func addFacets(fc *ListFacets, rows []listRowOf) []IncidentListItem {
	matched := make([]IncidentListItem, 0, len(rows))
	byNS, byKind, kindName := map[string]int{}, map[string]int{},
		map[string]string{}
	for _, r := range rows {
		item := r.item
		if r.passesBut(passSeverity) {
			fc.Severity[item.Severity]++
		}
		if r.passesBut(passStatus) {
			fc.State[item.State]++
		}
		if r.passesBut(passNamespace) && item.Namespace != "" {
			byNS[item.Namespace]++
		}
		if r.passesBut(passKind) && item.WorkloadKind != "" {
			key := strings.ToLower(item.WorkloadKind)
			byKind[key]++
			if _, ok := kindName[key]; !ok {
				kindName[key] = item.WorkloadKind
			}
		}
		if r.passesBut(passAcked) {
			fc.Acked[strconv.FormatBool(item.AckedBy != "")]++
		}
		if r.passesBut(passStopped) {
			fc.Stopped[strconv.FormatBool(item.StoppedTracking)]++
		}
		if r.pass == passAll {
			matched = append(matched, item)
		}
	}
	fc.Namespace = topCounts(byNS, nil, maxNamespaceFacets)
	fc.Kind = topCounts(byKind, kindName, maxKindFacets)
	return matched
}

// topCounts is the limit names with the most issues (ties A to Z) and
// their counts. names maps a key to the name shown; nil shows the key.
func topCounts(by map[string]int, names map[string]string,
	limit int) map[string]int {
	keys := make([]string, 0, len(by))
	for key := range by {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		if c := cmp.Compare(by[b], by[a]); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	out := make(map[string]int, min(len(keys), limit))
	for _, key := range keys[:min(len(keys), limit)] {
		name := key
		if names != nil {
			name = names[key]
		}
		out[name] = by[key]
	}
	return out
}
