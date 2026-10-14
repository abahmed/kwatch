package pagerduty

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Events API v2 request for the canonical
// messages. Update with: go test -run 'TestContract/resolve$' -update.
func TestContract(t *testing.T) {
	c, rec := newTestPagerDuty(t)
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"rk"}, LimitBytes: 20000,
		KeyOf: func(t *testing.T, r providertest.Request) string {
			return r.JSON(t)["dedup_key"].(string)
		},
		Check: checkEventShape,
	})
}

func checkEventShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if name == "digest" {
		if len(reqs) != 0 {
			t.Fatal("a digest must not page")
		}
		return
	}
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	var p pagerdutyPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	if p.RoutingKey != "rk" || p.DedupKey == "" {
		t.Fatalf("routing or dedup key missing: %+v", p)
	}
	if name == "resolve" {
		if p.EventAction != "resolve" || p.Payload != nil {
			t.Fatalf("resolve = %+v", p)
		}
		return
	}
	d := p.Payload
	if p.EventAction != "trigger" || d == nil || d.Summary == "" ||
		d.Source == "" || d.Severity != "critical" {
		t.Fatalf("trigger lacks required fields: %+v", p)
	}
	if utf8.RuneCountInString(d.Summary) > summaryLimit {
		t.Fatalf("summary is %d runes", utf8.RuneCountInString(d.Summary))
	}
}
