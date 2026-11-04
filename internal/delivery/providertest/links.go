package providertest

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/notification"
)

// The dashboard links of Linked, as compose writes them: each label
// links its address, and plain text reads the line "label: address".
const (
	CauseLabel    = "Cause in dashboard"
	CauseURL      = "https://kwatch.example/issues/p-cause"
	EarlierLabel  = "Earlier times"
	EarlierURL    = "https://kwatch.example/issues?status=all&like=p-link"
	IncidentLabel = "Open in dashboard"
	IncidentURL   = "https://kwatch.example/issues/p-link"
)

// LinkSpans are the link spans of Linked, in order.
func LinkSpans() []notification.Span {
	return []notification.Span{
		notification.LinkSpan(CauseLabel, CauseURL),
		notification.LinkSpan(EarlierLabel, EarlierURL),
		notification.LinkSpan(IncidentLabel, IncidentURL),
	}
}

// linkLeads are the parts of the links every rendering keeps: the
// labels, and the two addresses that have no character any markup
// escapes.
var linkLeads = []string{CauseLabel, EarlierLabel, IncidentLabel,
	"kwatch.example/issues/p-cause", "kwatch.example/issues/p-link"}

// Linked is an announcement whose blocks end with the three dashboard
// links, one per line, as a message reads when the Pro dashboard is
// exposed.
func Linked() notification.Message {
	m := withKey(Announce(), "p-link")
	m.Doc = []notification.Block{
		{Kind: notification.Para, Spans: []notification.Span{
			{Text: m.Short}}},
		{Kind: notification.Para, Spans: []notification.Span{
			{Text: "Caused by: Node n1 is not ready."}}},
	}
	// The plain Note is written before the links are added, so it never
	// carries one.
	m.Note = m.NoteText()
	for _, link := range LinkSpans() {
		m.Doc = append(m.Doc, notification.Block{Kind: notification.Para,
			Spans: []notification.Span{link}})
	}
	return m
}

// AssertDashboardLinks checks the requests of the linked case: chat, mail,
// paging details and issue bodies carry every link, while SMS and push
// texts (the Short) and the structured Note carry none.
func AssertDashboardLinks(t *testing.T, reqs []Request, want bool) {
	t.Helper()
	if len(reqs) == 0 {
		t.Fatal("the linked message was not sent")
	}
	var all []string
	for _, r := range reqs {
		all = append(all, visibleStrings(r)...)
	}
	text := strings.Join(all, "\n")
	for _, lead := range linkLeads {
		if got := strings.Contains(text, lead); got != want {
			t.Fatalf("dashboard link %q present = %v, want %v",
				lead, got, want)
		}
	}
}
