package zenduty

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Zenduty events API request for the
// canonical messages. The integration key is part of the URL path.
func TestContract(t *testing.T) {
	c, rec := newTestZenduty(t, nil)
	c.integrationkey = "ik-zenduty-secret"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets:    []string{"ik-zenduty-secret"},
		LimitBytes: safetext.DetailsLimit,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["entity_id"].(string)
		},
		Check: checkEventShape,
	})
}

func checkEventShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not open an alert")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/ik-zenduty-secret/" {
		t.Fatalf("got %d requests", len(reqs))
	}
	var p zendutyPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	want := "critical"
	if name == "resolve" {
		want = "resolved"
	}
	if p.AlertType != want || p.EntityID == "" || p.Message == "" {
		t.Fatalf("event = %+v, want alert_type %s", p, want)
	}
	if len(p.Message) > maxMessageBytes ||
		len(p.Summary) > safetext.DetailsLimit+64 {
		t.Fatalf("limits exceeded: %d/%d", len(p.Message), len(p.Summary))
	}
	providertest.AssertFencesBalanced(t, p.Summary)
}
