package pagerduty

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newTestPagerDuty(t *testing.T) (*Pagerduty, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewPagerDuty(map[string]interface{}{"integrationKey": "rk"},
		"dev", rec.Dependencies())
	c.url = rec.URL()
	return c, rec
}

func TestPagerDutyConfigValidation(t *testing.T) {
	deps := providertest.NewRecorder(t).Dependencies()
	if NewPagerDuty(map[string]interface{}{}, "dev", deps) != nil {
		t.Fatal("missing integration key must be rejected")
	}
	c := NewPagerDuty(map[string]interface{}{"integrationKey": "k"},
		"dev", deps)
	if c == nil || c.Name() != "PagerDuty" || c.url != pagerdutyAPIURL {
		t.Fatalf("provider = %+v", c)
	}
}

func TestPagerDutyIncidentLifecycle(t *testing.T) {
	c, rec := newTestPagerDuty(t)
	wantAction := map[string]string{
		"announce": "trigger", "update": "trigger", "resolve": "resolve",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			if err := c.SendIncident(context.Background(), m); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			var p pagerdutyPayload
			if err := json.Unmarshal(rec.Last(t).Body, &p); err != nil {
				t.Fatal(err)
			}
			if p.DedupKey != "kwatch-dev-p-42" || p.RoutingKey != "rk" ||
				p.EventAction != wantAction[tc.Name] {
				t.Fatalf("payload = %+v", p)
			}
			if m.Resolved() {
				if p.Payload != nil {
					t.Fatal("resolve must not carry a payload")
				}
				return
			}
			d := p.Payload
			if d.Summary != m.Short || d.Severity != "critical" ||
				d.Source != "dev" || d.CustomDetail.Details != m.Note {
				t.Fatalf("detail = %+v", d)
			}
			providertest.AssertOneLeadingEmoji(t, d.Summary)
		})
	}
}

func TestPagerDutySummaryIsTruncated(t *testing.T) {
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("é", 1000)
	p := (&Pagerduty{}).buildPayload(m)
	if len(p.Payload.Summary) > summaryLimit {
		t.Fatalf("summary = %d bytes", len(p.Payload.Summary))
	}
}

func TestPagerDutySkipsNotices(t *testing.T) {
	c, rec := newTestPagerDuty(t)
	providertest.AssertNoticesSkipped(t, c, rec)
}
