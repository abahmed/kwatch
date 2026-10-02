package splunkoncall

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newTestSplunkOncall(
	t *testing.T,
) (*SplunkOncall, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewSplunkOncall(map[string]interface{}{
		"apiKey": "k", "routingKey": "r",
	}, "dev", rec.Dependencies())
	c.url = rec.URL()
	return c, rec
}

func decodePayload(t *testing.T, body []byte) splunkOnCallPayload {
	t.Helper()
	var p splunkOnCallPayload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSplunkOncallIncidentLifecycle(t *testing.T) {
	c, rec := newTestSplunkOncall(t)
	want := map[string]string{
		"announce": "CRITICAL", "update": "CRITICAL", "resolve": "RECOVERY",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			if err := c.SendIncident(context.Background(), m); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			p := decodePayload(t, rec.Last(t).Body)
			if p.MessageType != want[tc.Name] ||
				p.EntityID != "kwatch-dev-p-42" ||
				p.EntityDisplayName != m.Short ||
				!strings.HasPrefix(p.StateMessage, m.Note) {
				t.Fatalf("payload = %+v", p)
			}
			providertest.AssertOneLeadingEmoji(t, p.EntityDisplayName)
		})
	}
}

func TestSplunkOncallTitleIsTruncated(t *testing.T) {
	c, rec := newTestSplunkOncall(t)
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 400)
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	p := decodePayload(t, rec.Last(t).Body)
	if len(p.EntityDisplayName) > titleLimit {
		t.Fatalf("title = %d bytes", len(p.EntityDisplayName))
	}
}

func TestSplunkOncallSendMessageIsInfoNotice(t *testing.T) {
	c, rec := newTestSplunkOncall(t)
	if err := c.SendMessage(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	p := decodePayload(t, rec.Last(t).Body)
	if p.MessageType != "INFO" || p.EntityID != "kwatch-dev-notice" ||
		p.EntityDisplayName != "hello" || p.StateMessage != "hello" {
		t.Fatalf("payload = %+v", p)
	}
}

func TestSplunkOncallRejectedRequestFails(t *testing.T) {
	c, rec := newTestSplunkOncall(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	if err := c.SendMessage(context.Background(), "x"); err == nil {
		t.Fatal("expected an error for 401")
	}
	c.url = "h ttp://localhost/%s"
	if err := c.SendMessage(context.Background(), "x"); err == nil {
		t.Fatal("expected an error for an invalid URL")
	}
}
