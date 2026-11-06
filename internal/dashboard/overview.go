package dashboard

import (
	"cmp"
	"net/http"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// overviewTop bounds the issues "Look at these first" lists.
const overviewTop = 5

// OverviewResponse is the /api/overview response: the Overview's top
// in one read. Counts are the issues not resolved by severity;
// LookFirst the issues to open first; FailedSendsLastHour the provider
// sends that failed or are retried in the last hour, absent without
// delivery (as failedLastHour on /api/status).
type OverviewResponse struct {
	Counts              SeverityCounts  `json:"counts"`
	LookFirst           []OverviewIssue `json:"lookFirst"`
	FailedSendsLastHour *int            `json:"failedSendsLastHour,omitempty"`
	// Releases is the last 24 hours of releases; absent until the
	// release book is read.
	Releases *ReleaseSummary `json:"releases,omitempty"`
}

// SeverityCounts count issues by severity in the page's words: urgent
// (page), attention (notify), low (digest) and quiet (silent).
type SeverityCounts struct {
	Urgent    int `json:"urgent"`
	Attention int `json:"attention"`
	Low       int `json:"low"`
	Quiet     int `json:"quiet"`
}

// add counts one issue at tier.
func (c *SeverityCounts) add(t incident.Tier) {
	switch t {
	case incident.Page:
		c.Urgent++
	case incident.Notify:
		c.Attention++
	case incident.Digest:
		c.Low++
	default:
		c.Quiet++
	}
}

// OverviewIssue is one issue to look at: Kind and Name are its root
// object's, Since when it started, Acked whether someone acknowledged
// it.
type OverviewIssue struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Namespace string    `json:"namespace,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Name      string    `json:"name,omitempty"`
	Severity  string    `json:"severity"`
	State     string    `json:"state"`
	Since     time.Time `json:"since"`
	Acked     bool      `json:"acked,omitempty"`
}

// overviewHandler serves GET /api/overview.
func (s *Server) overviewHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
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
	out := buildOverview(items, s.texts.redact)
	out.FailedSendsLastHour = s.failedLastHour()
	if snap, ok := s.releaseBookSnapshot(); ok {
		out.Releases = summarizeReleases(snap,
			s.now.Now().Add(-releaseWindow))
	}
	writeTagged(w, r, out)
}

// buildOverview counts the issues not resolved and picks the ones to
// look at first.
func buildOverview(
	items []incident.Incident, red func(string) string,
) OverviewResponse {
	if red == nil {
		red = func(s string) string { return s }
	}
	out := OverviewResponse{LookFirst: make([]OverviewIssue, 0)}
	var live []incident.Incident
	for _, p := range items {
		if p.State == incident.Resolved {
			continue
		}
		out.Counts.add(p.Tier)
		if activeIssue(p) {
			live = append(live, p)
		}
	}
	slices.SortFunc(live, lookFirst)
	for _, p := range live[:min(len(live), overviewTop)] {
		out.LookFirst = append(out.LookFirst, overviewIssue(p, red))
	}
	return out
}

// lookFirst orders the urgent issues nobody acknowledged first, then
// the newest started, then by ID.
func lookFirst(a, b incident.Incident) int {
	return cmp.Or(cmp.Compare(rankOf(a), rankOf(b)),
		b.Opened.Compare(a.Opened), cmp.Compare(a.ID, b.ID))
}

func rankOf(p incident.Incident) int {
	if p.Tier == incident.Page && p.Ack == nil {
		return 0
	}
	return 1
}

func overviewIssue(
	p incident.Incident, red func(string) string,
) OverviewIssue {
	title, _ := shortText(titleFor(p, red), maxInsightText)
	return OverviewIssue{ID: p.ID, Title: title,
		Namespace: p.Root.Namespace, Kind: string(p.Root.Kind),
		Name: p.Root.Name, Severity: p.Tier.String(),
		State: p.State.String(), Since: p.Opened, Acked: p.Ack != nil}
}
