package incidentio

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact incident.io alert event request for the
// canonical messages.
func TestContract(t *testing.T) {
	c, rec := newTestIncidentio(t)
	// "key" would also match the deduplication_key field name.
	c.apiKey = "tok-incidentio"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{"tok-incidentio"}, LimitBytes: safetext.DetailsLimit,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["deduplication_key"].(string)
		},
		Check: checkEventShape,
	})
}

func checkEventShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not fire an alert")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	if reqs[0].Header.Get("Authorization") != "Bearer tok-incidentio" {
		t.Fatal("missing bearer authorization")
	}
	var p incidentioPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	want := "firing"
	if name == "resolve" {
		want = "resolved"
	}
	if p.Status != want || p.Title == "" || p.DeduplicationKey == "" {
		t.Fatalf("event = %+v, want status %s", p, want)
	}
	if len(p.Title) > titleLimit || len(p.Description) >
		safetext.DetailsLimit {
		t.Fatalf("limits exceeded: %d/%d", len(p.Title),
			len(p.Description))
	}
	providertest.AssertFencesBalanced(t, p.Description)
}
