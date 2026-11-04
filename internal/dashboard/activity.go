package dashboard

import (
	"cmp"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// maxActivity bounds the events /api/activity returns: the newest.
const maxActivity = 20

// activityWindows are the ?since= values /api/activity accepts; empty
// is the first.
var activityWindows = map[string]time.Duration{
	"":    time.Hour,
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
}

// Activity event kinds, in the order events of one moment are listed.
const (
	activityNew      = "new"
	activityUrgent   = "urgent"
	activityReopened = "reopened"
	activityResolved = "resolved"
)

var activityOrder = map[string]int{activityNew: 0, activityUrgent: 1,
	activityReopened: 2, activityResolved: 3}

// ActivityResponse is the /api/activity response: what changed in the
// window, newest first, at most maxActivity events. Total counts every
// event in the window, so the page can say more happened.
type ActivityResponse struct {
	Since  string          `json:"since"`
	Events []ActivityEvent `json:"events"`
	Total  int             `json:"total"`
}

// ActivityEvent is one change to one incident: it opened (new), became
// more urgent (its tier rose), came back (reopened or recurred) or
// resolved.
// The other fields are the incident as it is now.
type ActivityEvent struct {
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Severity  string    `json:"severity"`
	State     string    `json:"state"`
	Namespace string    `json:"namespace"`
	Workload  string    `json:"workload"`
	// WorkloadKind is the root object's kind, as the list shows it.
	WorkloadKind string `json:"workloadKind,omitempty"`
	// root and own find the copies of one event: a recurrence carries
	// the opens and resolves of the incident it repeats. own marks the
	// copy of the incident the event happened to.
	root string
	own  bool
}

// activityHandler serves GET /api/activity?since=1h|24h, with an ETag
// like the list.
func (s *Server) activityHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	since := r.URL.Query().Get("since")
	window, ok := activityWindows[since]
	if !ok || len(r.URL.Query()["since"]) > 1 {
		writeBadQuery(w, badQuery("since", since,
			"Unknown activity window: %q. Use 1h or 24h.", since))
		return
	}
	reader := s.sources.incidents.Load()
	if reader == nil {
		unavailable(w)
		return
	}
	out := buildActivity((*reader).Incidents(), s.now.Now(), window,
		s.texts.redact)
	out.Since = cmp.Or(since, "1h")
	writeTagged(w, r, out)
}

// buildActivity lists the events of items within window before now,
// newest first; ties fall back to the incident ID and the kind, so two
// reads of the same incidents give the same body. Quiet incidents are
// left out, as on the active list.
func buildActivity(items []incident.Incident, now time.Time,
	window time.Duration, red func(string) string) ActivityResponse {
	from := now.Add(-window)
	in := func(t time.Time) bool {
		return !t.IsZero() && t.After(from) && !t.After(now)
	}
	events := make([]ActivityEvent, 0)
	for i, p := range items {
		if i >= maxSimilarScan {
			break
		}
		if p.Tier == incident.Silent {
			continue
		}
		events = appendActivity(events, p, in, red)
	}
	events = dropCopies(events)
	slices.SortFunc(events, func(a, b ActivityEvent) int {
		return cmp.Or(b.At.Compare(a.At), cmp.Compare(a.ID, b.ID),
			cmp.Compare(activityOrder[a.Kind], activityOrder[b.Kind]))
	})
	return ActivityResponse{Events: events[:min(len(events), maxActivity)],
		Total: len(events)}
}

// appendActivity adds p's events that in keeps.
func appendActivity(events []ActivityEvent, p incident.Incident,
	in func(time.Time) bool, red func(string) string) []ActivityEvent {
	var item *IncidentListItem
	add := func(at time.Time, kind string, own bool) {
		if !in(at) {
			return
		}
		if item == nil {
			v := listItem(p, red)
			item = &v
		}
		events = append(events, ActivityEvent{At: at, Kind: kind,
			ID: item.ID, Title: item.Title, Severity: item.Severity,
			State: item.State, Namespace: item.Namespace,
			Workload: item.Workload, WorkloadKind: item.WorkloadKind,
			root: p.Root.String(), own: own})
	}
	for i, at := range openings(p) {
		add(at, openingKind(p, i, at), !at.Before(p.Opened))
	}
	for _, at := range urgentTimes(p) {
		add(at, activityUrgent, !at.Before(p.Opened))
	}
	for _, o := range p.History {
		add(o.Resolved, activityResolved, false)
	}
	// A superseded incident closes because another took its failures
	// over: nothing was fixed.
	if p.SupersededBy == "" {
		add(p.Resolved, activityResolved, true)
	}
	return events
}

// urgentTimes are when p became more urgent: every tier rise its
// journal recorded and, before the journal's first entry (an incident
// saved by an older kwatch), every "paging again" in its timeline.
func urgentTimes(p incident.Incident) []time.Time {
	var out []time.Time
	var since time.Time
	for i, t := range p.Journal.Transitions {
		if i == 0 {
			since = t.At
		}
		if t.Kind == incident.TransitionTierRaised {
			out = append(out, t.At)
		}
	}
	for _, e := range p.Timeline {
		if !since.IsZero() && !e.At.Before(since) {
			continue
		}
		if kind, _ := classifyEvent(e.Text); kind == kindPagedAgain {
			out = append(out, e.At)
		}
	}
	return out
}

// dropCopies keeps one event per moment, root and change: the copy of
// the incident it happened to, else the first by ID.
func dropCopies(events []ActivityEvent) []ActivityEvent {
	// A new and a reopened copy are the same opening.
	class := func(e ActivityEvent) string {
		if e.Kind == activityReopened {
			return activityNew
		}
		return e.Kind
	}
	slices.SortFunc(events, func(a, b ActivityEvent) int {
		return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.root, b.root),
			strings.Compare(class(a), class(b)), boolFirst(a.own, b.own),
			cmp.Compare(activityOrder[a.Kind], activityOrder[b.Kind]),
			cmp.Compare(a.ID, b.ID))
	})
	return slices.CompactFunc(events, func(a, b ActivityEvent) bool {
		return a.At.Equal(b.At) && a.root == b.root && class(a) == class(b)
	})
}

// boolFirst orders true before false.
func boolFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

// openingKind is "new" for the first time an incident opened and
// "reopened" for every later time, a recurrence of an earlier incident
// included. The earliest opening kept is the first unless an earlier
// occurrence is remembered.
func openingKind(p incident.Incident, i int, at time.Time) string {
	if i == 0 && p.Previous == "" &&
		(len(p.History) == 0 || !p.History[0].Opened.Before(at)) {
		return activityNew
	}
	return activityReopened
}
