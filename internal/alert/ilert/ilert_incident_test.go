package ilert

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

func newTestIlert(t *testing.T) (*Ilert, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewIlert(map[string]interface{}{"integrationKey": "ik"}, "dev",
		rec.Dependencies())
	c.url = rec.URL()
	return c, rec
}

func decodePayload(t *testing.T, body []byte) ilertPayload {
	t.Helper()
	var p ilertPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIlertIncidentLifecycle(t *testing.T) {
	c, rec := newTestIlert(t)
	want := map[string]string{
		"announce": "ALERT", "update": "ALERT", "resolve": "RESOLVE",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			if err := c.SendIncident(context.Background(), m); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			p := decodePayload(t, rec.Last(t).Body)
			if p.EventType != want[tc.Name] || p.AlertKey != "kwatch-dev-p-42" ||
				p.Summary != m.Short || p.Message != m.Note ||
				!strings.HasPrefix(p.Details, m.Note) ||
				p.Priority != "HIGH" {
				t.Fatalf("payload = %+v", p)
			}
			providertest.AssertOneLeadingEmoji(t, p.Summary)
		})
	}
}

func TestIlertSummaryIsTruncated(t *testing.T) {
	c, rec := newTestIlert(t)
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 400)
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if p := decodePayload(t, rec.Last(t).Body); len(p.Summary) > summaryLimit {
		t.Fatalf("summary = %d bytes", len(p.Summary))
	}
}

func TestIlertSkipsNotices(t *testing.T) {
	c, rec := newTestIlert(t)
	providertest.AssertNoticesSkipped(t, c, rec)
}

// "Severity follows the incident severity" (docs/providers.md): only a
// page-tier incident gets the priority that pages.
func TestIlertPriorityFollowsTheIncidentSeverity(t *testing.T) {
	cases := []struct {
		name     string
		severity string
		status   notification.Status
		want     string
	}{
		{"page tier", "critical", notification.StatusCritical, "HIGH"},
		{"notify tier", "warning", notification.StatusWarning, "LOW"},
		{"low tier", "info", notification.StatusWarning, "LOW"},
		{"warning route on a critical status", "warning",
			notification.StatusCritical, "LOW"},
		{"no route, critical status", "", notification.StatusCritical,
			"HIGH"},
		{"no route, warning status", "", notification.StatusWarning, "LOW"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newTestIlert(t)
			m := providertest.Announce()
			m.Route.Severity, m.Status = tc.severity, tc.status
			if err := c.SendIncident(context.Background(), m); err != nil {
				t.Fatal(err)
			}
			if got := decodePayload(t, rec.Last(t).Body).Priority; got !=
				tc.want {
				t.Fatalf("priority = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIlertConfiguredPriorityIsForPagesOnly(t *testing.T) {
	c := &Ilert{priority: "CRITICAL"}
	page, note := providertest.Announce(), providertest.Announce()
	note.Route.Severity = "warning"
	if got := c.priorityFor(page); got != "CRITICAL" {
		t.Errorf("page priority = %q", got)
	}
	if got := c.priorityFor(note); got != "LOW" {
		t.Errorf("notify priority = %q", got)
	}
}
