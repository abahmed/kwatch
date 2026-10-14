package goalert

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/alert/safetext"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact GoAlert generic incoming request for the
// canonical messages.
func TestContract(t *testing.T) {
	c, rec := newTestGoalert(t)
	c.token = "tok-goalert"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{"tok-goalert"}, LimitBytes: safetext.DetailsLimit,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["dedup"].(string)
		},
		Check: checkAlertShape,
	})
}

func checkAlertShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not open an alert")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("got %d requests", len(reqs))
	}
	if reqs[0].Header.Get("Authorization") != "Bearer tok-goalert" {
		t.Fatal("missing bearer authorization")
	}
	var p goalertPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	if p.Dedup == "" || p.Summary == "" {
		t.Fatalf("dedup or summary missing: %+v", p)
	}
	wantAction := ""
	if name == "resolve" {
		wantAction = "close"
	}
	if p.Action != wantAction {
		t.Fatalf("action = %q, want %q", p.Action, wantAction)
	}
	if utf8.RuneCountInString(p.Summary) > summaryLimit ||
		len(p.Details) > safetext.DetailsLimit {
		t.Fatalf("limits exceeded: %d/%d", len(p.Summary),
			len(p.Details))
	}
	providertest.AssertFencesBalanced(t, p.Details)
}
