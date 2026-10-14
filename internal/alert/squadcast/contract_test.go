package squadcast

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Squadcast incident API request for the
// canonical messages. The test URL replaces the keyed path.
func TestContract(t *testing.T) {
	c, rec := newTestSquadcast(t)
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		LimitBytes: safetext.DetailsLimit,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["event_id"].(string)
		},
		Check: checkIncidentShape,
	})
}

func checkIncidentShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not open an incident")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	var p squadcastPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	want := "trigger"
	if name == "resolve" {
		want = "resolve"
	}
	if p.Status != want || p.Message == "" || p.EventID == "" ||
		p.Severity != "critical" {
		t.Fatalf("event = %+v, want status %s", p, want)
	}
	if len(p.Message) > messageLimit ||
		len(p.Description) > safetext.DetailsLimit {
		t.Fatalf("limits exceeded: %d/%d", len(p.Message),
			len(p.Description))
	}
	providertest.AssertFencesBalanced(t, p.Description)
}
