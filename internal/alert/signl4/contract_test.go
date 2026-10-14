package signl4

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact SIGNL4 webhook request for the canonical
// messages. The team secret is the last segment of the URL path.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSignl4(map[string]interface{}{
		"teamSecret": "ts-signl4-secret", "url": rec.URL(),
		"user": "kwatch",
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets:    []string{"ts-signl4-secret"},
		LimitBytes: safetext.DetailsLimit,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["X-S4-ExternalID"].(string)
		},
		Check: checkAlertShape,
	})
}

func checkAlertShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not raise an alert")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/ts-signl4-secret" {
		t.Fatalf("got %d requests", len(reqs))
	}
	var p signl4Payload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	want := "new"
	if name == "resolve" {
		want = "resolved"
	}
	if p.XS4Status != want || p.ExternalID == "" || p.Title == "" ||
		p.Severity != "critical" {
		t.Fatalf("alert = %+v, want status %s", p, want)
	}
	if len(p.Title) > maxTitleBytes ||
		len(p.Message) > safetext.DetailsLimit+64 {
		t.Fatalf("limits exceeded: %d/%d", len(p.Title), len(p.Message))
	}
	providertest.AssertFencesBalanced(t, p.Message)
}
