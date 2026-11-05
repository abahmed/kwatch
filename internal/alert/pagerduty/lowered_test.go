package pagerduty

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// The update that lowers a page re-triggers the same alert (dedup key)
// with the lower severity, "warning". PagerDuty updates the alert; the
// incident it opened keeps the urgency it was given.
func TestPagerDutyLoweringUpdateLowersSeverity(t *testing.T) {
	c, rec := newTestPagerDuty(t)
	m := providertest.Update()
	m.Route.Severity = "warning"
	m.PagingOnly = true
	if err := c.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	var p pagerdutyPayload
	if err := json.Unmarshal(rec.Last(t).Body, &p); err != nil {
		t.Fatal(err)
	}
	if p.EventAction != "trigger" || p.DedupKey != "kwatch-dev-p-42" ||
		p.Payload == nil || p.Payload.Severity != "warning" {
		t.Fatalf("payload = %+v", p)
	}
}
