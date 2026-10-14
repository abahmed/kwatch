package splunkoncall

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Splunk On-Call REST endpoint request for
// the canonical messages, including the api key and routing key path.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSplunkOncall(map[string]interface{}{
		"apiKey": "ak-splunkoncall", "routingKey": "everyone",
		"url": rec.URL(),
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets:    []string{"ak-splunkoncall"},
		LimitBytes: safetext.DetailsLimit,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["entity_id"].(string)
		},
		Check: checkAlertShape,
	})
}

func checkAlertShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not open an incident")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("got %d requests", len(reqs))
	}
	if reqs[0].Path != "/ak-splunkoncall/everyone" {
		t.Fatalf("path = %q, want /<api key>/<routing key>", reqs[0].Path)
	}
	var p splunkOnCallPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	want := "CRITICAL"
	if name == "resolve" {
		want = "RECOVERY"
	}
	if p.MessageType != want || p.EntityID == "" ||
		p.EntityDisplayName == "" {
		t.Fatalf("alert = %+v, want type %s", p, want)
	}
	if utf8.RuneCountInString(p.EntityDisplayName) > titleLimit ||
		len(p.StateMessage) > safetext.DetailsLimit {
		t.Fatalf("limits exceeded: %d/%d", len(p.EntityDisplayName),
			len(p.StateMessage))
	}
	providertest.AssertFencesBalanced(t, p.StateMessage)
}
