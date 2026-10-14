package splunk

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact HTTP Event Collector request for the
// canonical messages. Splunk is a log sink, so even a digest is indexed.
func TestContract(t *testing.T) {
	c, rec := newTestSplunk(t)
	c.token = "tok-splunk-hec"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{"tok-splunk-hec"},
		KeyOf: func(t *testing.T, r providertest.Request) string {
			ev := r.JSON(t)["event"].(map[string]any)
			return ev["incidentKey"].(string)
		},
		Check: checkEventShape,
	})
}

func checkEventShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/services/collector/event" {
		t.Fatalf("got %d requests", len(reqs))
	}
	if reqs[0].Header.Get("Authorization") != "Splunk tok-splunk-hec" {
		t.Fatal("missing Splunk authorization")
	}
	body := reqs[0].JSON(t)
	ev, ok := body["event"].(map[string]any)
	if !ok || body["index"] != "k8s" ||
		body["sourcetype"] != "kwatch:incident" {
		t.Fatalf("event envelope = %v", body)
	}
	want := "firing"
	if name == "resolve" {
		want = "resolved"
	}
	if ev["state"] != want || ev["incidentKey"] == "" ||
		ev["cluster"] != "dev" || ev["title"] == "" {
		t.Fatalf("event = %v, want state %s", ev, want)
	}
}
