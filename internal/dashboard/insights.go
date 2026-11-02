package dashboard

import (
	"cmp"
	"net/http"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// Insight bounds: the issues one card lists, the incidents one answer
// reads and the namespace tiles of the map.
const (
	insightTop      = 5
	maxInsightScan  = 20_000
	maxInsightTiles = 60
	// insightChangeWindow is how long before an issue started a change
	// still counts as "right before" it.
	insightChangeWindow = 30 * time.Minute
	// insightDeliveryWindow is how far back failed sends are listed.
	insightDeliveryWindow = 24 * time.Hour
	// maxInsightText bounds a free-text field (a title, an error).
	maxInsightText = 200
)

// InsightsResponse is the /api/insights response: the Overview's short
// cards, each empty when there is nothing to say.
type InsightsResponse struct {
	Today       InsightToday      `json:"today"`
	Unacked     InsightIssues     `json:"unacked"`
	Nodes       []InsightNode     `json:"nodes"`
	AfterChange InsightChanges    `json:"afterChange"`
	Undelivered InsightDeliveries `json:"undelivered"`
	Causes      []InsightCause    `json:"causes"`
	Namespaces  InsightNamespaces `json:"namespaces"`
	Quiet       *InsightQuiet     `json:"quiet,omitempty"`
}

// InsightToday counts today's issues (in the requested zone): opened,
// resolved, and urgent ones still active now.
type InsightToday struct {
	New      int `json:"new"`
	Resolved int `json:"resolved"`
	Urgent   int `json:"urgent"`
}

// InsightIssue is one issue a card links to. Since is when it started.
type InsightIssue struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Severity  string    `json:"severity"`
	State     string    `json:"state"`
	Namespace string    `json:"namespace,omitempty"`
	Workload  string    `json:"workload,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Since     time.Time `json:"since"`
}

// InsightIssues is a count and the first issues of it.
type InsightIssues struct {
	Count  int            `json:"count"`
	Issues []InsightIssue `json:"issues"`
}

// activeIssue reports whether p is an issue the Overview counts: not
// resolved and not quiet.
func activeIssue(p incident.Incident) bool {
	return p.State != incident.Resolved && p.Tier != incident.Silent
}

// insightsHandler serves GET /api/insights?tz=, tz as on /api/trends.
func (s *Server) insightsHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	tz, ok := parseTZ(r.URL.Query().Get("tz"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid tz")
		return
	}
	reader := s.sources.incidents.Load()
	if reader == nil {
		unavailable(w)
		return
	}
	items := (*reader).Incidents()
	if len(items) > maxInsightScan {
		items = items[:maxInsightScan]
	}
	in := insightInput{items: items, now: s.now.Now(), tz: tz,
		red: s.texts.redact}
	if m := s.sources.model.Load(); m != nil {
		in.model = *m
	}
	if t := s.sources.trail.Load(); t != nil {
		in.trail = *t
	}
	out := buildInsights(in)
	if out.Quiet != nil {
		s.explainQuiet(out.Quiet, in.model)
	}
	writeTagged(w, r, out)
}

// insightInput is what the insights are computed from.
type insightInput struct {
	items []incident.Incident
	now   time.Time
	tz    int
	red   func(string) string
	model inventory.Reader
	trail deliveryTrailSource
}

// buildInsights computes every card; the quiet line is only set up
// here (explainQuiet fills it from the server's sources).
func buildInsights(in insightInput) InsightsResponse {
	if in.red == nil {
		in.red = func(s string) string { return s }
	}
	var live []incident.Incident
	for _, p := range in.items {
		if activeIssue(p) {
			live = append(live, p)
		}
	}
	slices.SortFunc(live, olderFirst)
	out := InsightsResponse{
		Today:       today(in.items, live, in.now, in.tz),
		Unacked:     unackedUrgent(live, in.red),
		Nodes:       busyNodes(in.model, live),
		AfterChange: afterChange(live, in.red),
		Undelivered: undelivered(in.items, in.trail, in.now, in.red),
		Causes:      topCauses(in.items),
		Namespaces:  namespaceMap(live),
	}
	if len(live) == 0 {
		out.Quiet = &InsightQuiet{}
	}
	return out
}

// olderFirst orders issues by when they started, then by ID.
func olderFirst(a, b incident.Incident) int {
	return cmp.Or(a.Opened.Compare(b.Opened), cmp.Compare(a.ID, b.ID))
}

// today counts openings and resolves within today in the tz zone, and
// the urgent issues active now.
func today(
	items, live []incident.Incident, now time.Time, tz int,
) InsightToday {
	w := newTrendWeek(now, tz)
	start := w.end.AddDate(0, 0, -1)
	in := func(t time.Time) bool {
		return !t.Before(start) && !t.After(now)
	}
	var out InsightToday
	for _, p := range items {
		if p.Tier == incident.Silent {
			continue
		}
		for _, at := range openings(p) {
			if in(at) {
				out.New++
			}
		}
		for _, o := range p.History {
			if in(o.Resolved) {
				out.Resolved++
			}
		}
		if p.State == incident.Resolved && p.SupersededBy == "" &&
			in(p.Resolved) {
			out.Resolved++
		}
	}
	for _, p := range live {
		if p.Tier == incident.Page {
			out.Urgent++
		}
	}
	return out
}

// insightIssues is the first n of list as card entries.
func insightIssues(
	list []incident.Incident, n int, red func(string) string,
) []InsightIssue {
	out := make([]InsightIssue, 0, min(n, len(list)))
	for _, p := range list[:min(n, len(list))] {
		out = append(out, insightIssue(p, red))
	}
	return out
}

func insightIssue(p incident.Incident, red func(string) string) InsightIssue {
	title, _ := shortText(titleFor(p, red), maxInsightText)
	return InsightIssue{ID: p.ID, Title: title,
		Severity: p.Tier.String(), State: p.State.String(),
		Namespace: p.Root.Namespace, Workload: p.Root.Name,
		Kind: string(p.Root.Kind), Since: p.Opened}
}

// unackedUrgent are the urgent active issues nobody acknowledged,
// newest first: the Overview's one urgent list. live is oldest first.
func unackedUrgent(
	live []incident.Incident, red func(string) string,
) InsightIssues {
	var list []incident.Incident
	for _, p := range slices.Backward(live) {
		if p.Tier == incident.Page && p.Ack == nil {
			list = append(list, p)
		}
	}
	return InsightIssues{Count: len(list),
		Issues: insightIssues(list, insightTop, red)}
}
