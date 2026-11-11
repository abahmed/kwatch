package pagerduty

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A key longer than the Events API accepts is capped, and the trigger and
// the resolve of one incident still carry the same key.
func TestPagerDutyDedupKeyIsCappedAndStable(t *testing.T) {
	c, rec := newTestPagerDuty(t)
	keys := map[string]bool{}
	for _, m := range []string{"trigger", "resolve"} {
		msg := providertest.Update()
		msg.Key = strings.Repeat("k", 400)
		if m == "resolve" {
			msg = providertest.Resolve()
			msg.Key = strings.Repeat("k", 400)
		}
		if err := c.SendIncident(context.Background(), msg); err != nil {
			t.Fatalf("SendIncident: %v", err)
		}
		var p pagerdutyPayload
		if err := json.Unmarshal(rec.Last(t).Body, &p); err != nil {
			t.Fatal(err)
		}
		if len(p.DedupKey) > 255 {
			t.Fatalf("dedup key is %d bytes", len(p.DedupKey))
		}
		keys[p.DedupKey] = true
	}
	if len(keys) != 1 {
		t.Fatalf("trigger and resolve keys differ: %v", keys)
	}
}
