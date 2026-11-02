package providertest

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/notification"
)

// The dashboard link lines of Linked, as compose writes them.
const (
	CauseLink   = "Cause in dashboard: https://kwatch.example/issues/p-cause"
	EarlierLink = "Earlier times: " +
		"https://kwatch.example/issues?status=all&like=p-link"
	IncidentLink = "Open in dashboard: https://kwatch.example/issues/p-link"
)

// linkLeads are the parts of the link lines every rendering keeps: the
// lead words, and the incident address, which has no character any
// markup escapes.
var linkLeads = []string{"Cause in dashboard:", "Earlier times:",
	"Open in dashboard:", "kwatch.example/issues/p-link"}

// Linked is an announcement whose blocks end with the three dashboard
// link lines, as a message reads when the Pro dashboard is exposed.
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
	for _, line := range []string{CauseLink, EarlierLink, IncidentLink} {
		m.Doc = append(m.Doc, notification.Block{Kind: notification.Para,
			Spans: []notification.Span{{Text: line}}})
	}
	return m
}

// AssertDashboardLinks checks the requests of the linked case: chat, mail,
// paging details and issue bodies carry every link line, while SMS and
// push texts (the Short) and the structured Note carry none.
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
