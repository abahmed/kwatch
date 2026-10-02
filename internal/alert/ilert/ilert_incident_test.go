package ilert

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
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
