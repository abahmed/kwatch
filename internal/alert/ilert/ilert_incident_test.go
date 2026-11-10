package ilert

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// testKey is distinctive so a redacted golden cannot mask other text.
const testKey = "ilert-key-123"

func newTestIlert(t *testing.T) (*Ilert, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewIlert(map[string]interface{}{"integrationKey": testKey}, "dev",
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
				p.Summary != m.Short || p.IntegrationKey != testKey ||
				!strings.HasPrefix(p.Details, m.Note) ||
				p.Priority != "HIGH" {
				t.Fatalf("payload = %+v", p)
			}
			providertest.AssertOneLeadingEmoji(t, p.Summary)
		})
	}
}

// The documented Events API is POST /api/events with the integration key
// in the JSON body. The old /api/v1/events/push/<key> form, with the key
// in the path, fails this test.
func TestIlertUsesTheDocumentedEventsAPI(t *testing.T) {
	c, rec := newTestIlert(t)
	u, err := url.Parse(ilertAPIURL)
	if err != nil || u.Host != "api.ilert.com" || u.Path != "/api/events" {
		t.Fatalf("ilertAPIURL = %q (%v)", ilertAPIURL, err)
	}
	c.url = rec.URL() + u.Path
	if err := c.SendIncident(
		context.Background(), providertest.Announce()); err != nil {
		t.Fatal(err)
	}
	req := rec.Last(t)
	if req.Method != "POST" || req.Path != "/api/events" {
		t.Fatalf("request = %s %s", req.Method, req.Path)
	}
	if strings.Contains(req.RawPath+req.Query, testKey) {
		t.Fatalf("integration key leaked into the URL: %s", req.RawPath)
	}
	body := req.JSON(t)
	if body["integrationKey"] != testKey {
		t.Fatalf("integrationKey = %v", body["integrationKey"])
	}
	if _, ok := body["message"]; ok {
		t.Fatal("undocumented message field is still sent")
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

func TestIlertConfiguredPriorityIsSentAsHighOrLow(t *testing.T) {
	for in, want := range map[interface{}]string{
		"CRITICAL": "HIGH", "critical": "HIGH", " High ": "HIGH",
		"low": "LOW", "bogus": "HIGH", "": "HIGH", nil: "HIGH", 3: "HIGH",
	} {
		if got := normalizePriority(in); got != want {
			t.Errorf("normalizePriority(%v) = %q, want %q", in, got, want)
		}
	}
	c := NewIlert(map[string]interface{}{
		"integrationKey": "k", "priority": "CRITICAL",
	}, testAppConfig(), testDeps)
	if got := c.priorityFor(providertest.Announce()); got != "HIGH" {
		t.Errorf("CRITICAL sent as %q, want HIGH", got)
	}
}

func TestIlertConfiguredPriorityIsForPagesOnly(t *testing.T) {
	c := &Ilert{priority: "HIGH"}
	page, note := providertest.Announce(), providertest.Announce()
	note.Route.Severity = "warning"
	if got := c.priorityFor(page); got != "HIGH" {
		t.Errorf("page priority = %q", got)
	}
	if got := c.priorityFor(note); got != "LOW" {
		t.Errorf("notify priority = %q", got)
	}
}
