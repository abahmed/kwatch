package squadcast

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newTestSquadcast(t *testing.T) (*Squadcast, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewSquadcast(map[string]interface{}{"serviceKey": "sk"}, "dev",
		rec.Dependencies())
	c.url = rec.URL()
	return c, rec
}

func decodePayload(t *testing.T, body []byte) squadcastPayload {
	t.Helper()
	var p squadcastPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSquadcastIncidentLifecycle(t *testing.T) {
	c, rec := newTestSquadcast(t)
	want := map[string]string{
		"announce": "trigger", "update": "trigger", "resolve": "resolve",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			if err := c.SendIncident(context.Background(), m); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			p := decodePayload(t, rec.Last(t).Body)
			if p.Status != want[tc.Name] || p.EventID != "kwatch-dev-p-42" ||
				p.Message != m.Short || p.Severity != "critical" ||
				!strings.HasPrefix(p.Description, m.Note) {
				t.Fatalf("payload = %+v", p)
			}
			providertest.AssertOneLeadingEmoji(t, p.Message)
		})
	}
}

func TestSquadcastMessageIsTruncated(t *testing.T) {
	c, rec := newTestSquadcast(t)
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 400)
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if p := decodePayload(t, rec.Last(t).Body); len(p.Message) > messageLimit {
		t.Fatalf("message = %d bytes", len(p.Message))
	}
}

func TestSquadcastSkipsNotices(t *testing.T) {
	c, rec := newTestSquadcast(t)
	providertest.AssertNoticesSkipped(t, c, rec)
}
