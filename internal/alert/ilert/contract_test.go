package ilert

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact iLert event API request for the
// canonical messages.
func TestContract(t *testing.T) {
	c, rec := newTestIlert(t)
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"ik"}, LimitBytes: safetext.DetailsLimit,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["alertKey"].(string)
		},
		Check: checkEventShape,
	})
}

func checkEventShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not raise an alert")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	var p ilertPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	want := "ALERT"
	if name == "resolve" {
		want = "RESOLVE"
	}
	if p.EventType != want || p.Summary == "" || p.AlertKey == "" ||
		p.Priority != "HIGH" {
		t.Fatalf("event = %+v, want type %s", p, want)
	}
	if utf8.RuneCountInString(p.Summary) > summaryLimit ||
		len(p.Details) > safetext.DetailsLimit {
		t.Fatalf("limits exceeded: %d/%d", len(p.Summary),
			len(p.Details))
	}
	providertest.AssertFencesBalanced(t, p.Details)
}
