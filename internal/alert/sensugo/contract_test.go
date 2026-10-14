package sensugo

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Sensu Go events API request for the
// canonical messages. The check name is the key; status 0 clears it.
func TestContract(t *testing.T) {
	c, rec := newTestSensugo(t)
	c.apiKey = "sensu-api-secret"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers:    []string{"Authorization"},
		Secrets:    []string{"sensu-api-secret"},
		LimitBytes: safetext.DetailsLimit,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			var p sensuPayload
			if err := json.Unmarshal(r.Body, &p); err != nil {
				t.Fatal(err)
			}
			return p.Entity.Metadata.Name + "|" + p.Check.Metadata.Name
		},
		Check: checkEventShape,
	})
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
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/api/core/v2/namespaces/default/events" {
		t.Fatalf("got %d requests", len(reqs))
	}
	if reqs[0].Header.Get("Authorization") != "Key sensu-api-secret" {
		t.Fatal("missing Key authorization")
	}
	var p sensuPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	want := 2
	if name == "resolve" {
		want = 0
	}
	if p.Check.Status != want || p.Check.Metadata.Name == "" ||
		p.Entity.Metadata.Name == "" || p.Check.Issued == 0 {
		t.Fatalf("event = %+v, want status %d", p, want)
	}
	if len(p.Check.Output) > safetext.DetailsLimit+64 {
		t.Fatalf("output is %d bytes", len(p.Check.Output))
	}
	providertest.AssertFencesBalanced(t, p.Check.Output)
}
