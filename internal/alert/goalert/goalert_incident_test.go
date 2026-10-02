package goalert

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newTestGoalert(t *testing.T) (*Goalert, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewGoalert(map[string]interface{}{
		"token": "tok", "serviceId": "SVC", "url": "https://goalert.corp",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	return c, rec
}

func decodePayload(t *testing.T, body []byte) goalertPayload {
	t.Helper()
	var p goalertPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGoalertIncidentLifecycle(t *testing.T) {
	c, rec := newTestGoalert(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			if err := c.SendIncident(context.Background(), m); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			req := rec.Last(t)
			if req.Header.Get("Authorization") != "Bearer tok" {
				t.Fatal("missing bearer token")
			}
			p := decodePayload(t, req.Body)
			wantAction := ""
			if m.Resolved() {
				wantAction = "close"
			}
			if p.Dedup != "kwatch-dev-p-42" || p.Action != wantAction ||
				p.Summary != m.Short || !strings.HasPrefix(p.Details, m.Note) {
				t.Fatalf("payload = %+v", p)
			}
			providertest.AssertOneLeadingEmoji(t, p.Summary)
		})
	}
}

func TestGoalertSummaryIsTruncated(t *testing.T) {
	c, rec := newTestGoalert(t)
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 400)
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if p := decodePayload(t, rec.Last(t).Body); len(p.Summary) > summaryLimit {
		t.Fatalf("summary = %d bytes", len(p.Summary))
	}
}

func TestGoalertSkipsNotices(t *testing.T) {
	c, rec := newTestGoalert(t)
	providertest.AssertNoticesSkipped(t, c, rec)
}
