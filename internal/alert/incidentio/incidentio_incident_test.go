package incidentio

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newTestIncidentio(t *testing.T) (*Incidentio, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewIncidentio(map[string]interface{}{
		"url": rec.URL(), "apiKey": "key",
	}, "dev", rec.Dependencies())
	return c, rec
}

func decodePayload(t *testing.T, body []byte) incidentioPayload {
	t.Helper()
	var p incidentioPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIncidentioIncidentLifecycle(t *testing.T) {
	c, rec := newTestIncidentio(t)
	want := map[string]string{
		"announce": "firing", "update": "firing", "resolve": "resolved",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			if err := c.SendIncident(context.Background(), m); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			req := rec.Last(t)
			if req.Header.Get("Authorization") != "Bearer key" {
				t.Fatal("missing bearer token")
			}
			p := decodePayload(t, req.Body)
			if p.Status != want[tc.Name] ||
				p.DeduplicationKey != "kwatch-dev-p-42" || p.Title != m.Short ||
				!strings.HasPrefix(p.Description, m.Note) ||
				p.Metadata["cluster"] != "dev" {
				t.Fatalf("payload = %+v", p)
			}
			providertest.AssertOneLeadingEmoji(t, p.Title)
		})
	}
}

func TestIncidentioTitleIsTruncated(t *testing.T) {
	c, rec := newTestIncidentio(t)
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 400)
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if p := decodePayload(t, rec.Last(t).Body); len(p.Title) > titleLimit {
		t.Fatalf("title = %d bytes", len(p.Title))
	}
}

func TestIncidentioSkipsNotices(t *testing.T) {
	c, rec := newTestIncidentio(t)
	providertest.AssertNoticesSkipped(t, c, rec)
}
