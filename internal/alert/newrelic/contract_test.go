package newrelic

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact New Relic Event API request for the
// canonical messages: a one-element array holding a KwatchAlert event.
func TestContract(t *testing.T) {
	c, rec := newTestNewRelic(t)
	c.apiKey = "nr-license-secret"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers:    []string{"Api-Key"},
		Secrets:    []string{"nr-license-secret"},
		LimitBytes: maxMessageBytes,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return eventOf(t, r)["incidentKey"].(string)
		},
		Check: checkEventShape,
	})
}

func eventOf(t *testing.T, r providertest.Request) map[string]any {
	var events []map[string]any
	if err := json.Unmarshal(r.Body, &events); err != nil ||
		len(events) != 1 {
		t.Fatalf("body is not a one-event array: %v\n%s", err, r.Body)
	}
	return events[0]
}

func checkEventShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not record an event")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("got %d requests", len(reqs))
	}
	if reqs[0].Header.Get("Api-Key") != "nr-license-secret" {
		t.Fatal("missing Api-Key header")
	}
	ev := eventOf(t, reqs[0])
	want := "firing"
	if name == "resolve" {
		want = "resolved"
	}
	if ev["eventType"] != "KwatchAlert" || ev["state"] != want ||
		ev["action"] != want || ev["incidentKey"] == "" {
		t.Fatalf("event = %v, want state %s", ev, want)
	}
	title, _ := ev["title"].(string)
	msg, _ := ev["message"].(string)
	if len(title) > maxTitleBytes || len(msg) > maxMessageBytes {
		t.Fatalf("limits exceeded: %d/%d", len(title), len(msg))
	}
	providertest.AssertFencesBalanced(t, msg)
}
