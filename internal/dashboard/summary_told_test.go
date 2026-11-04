package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSummaryToldKeepsTheLatestSendPerChannel(t *testing.T) {
	t1, t2 := testEpoch.Add(-time.Hour), testEpoch.Add(-time.Minute)
	d := IncidentDetail{Deliveries: []DeliveryAttempt{
		{At: t1, Channel: "slack", Result: "failed"},
		{At: t2, Channel: "slack", Result: "ok"},
		{At: t1, Channel: "email", Result: "retrying"},
		{At: t1, Channel: "webhook", Result: "failed"},
		{At: t2, Result: "carried"},
		{At: t2, Channel: "teams", Result: "not routed"},
	}}

	got := toldOf(&d)

	assert.Equal(t, []SummaryTold{
		{Name: "Email", Provider: "email", Kind: channelEmail,
			State: "pending", At: t1},
		{Name: "Slack", Provider: "slack", Kind: channelChat,
			State: "sent", At: t2},
		{Name: "Webhook", Provider: "webhook", Kind: channelOther,
			State: "failed", At: t1},
	}, got)
}

func TestSummaryToldTakesThePagerStateAndPutsPagersFirst(t *testing.T) {
	paged, closed := testEpoch.Add(-time.Hour), testEpoch
	d := IncidentDetail{
		Deliveries: []DeliveryAttempt{
			{At: paged, Channel: "pagerduty", Result: "ok", Pager: true},
			{At: paged, Channel: "custompager", Result: "ok",
				Pager: true},
			{At: paged, Channel: "slack", Result: "ok"},
		},
		DeliveryOutcome: DeliveryOutcome{Pager: []PagerRow{
			{Provider: "pagerduty", State: "closed", PagedAt: paged,
				ClosedAt: closed},
			{Provider: "opsgenie", State: "failed"},
		}},
	}

	got := toldOf(&d)

	assert.Equal(t, []SummaryTold{
		{Name: "Custompager", Provider: "custompager",
			Kind: channelPager, State: "sent", At: paged},
		{Name: "Opsgenie", Provider: "opsgenie", Kind: channelPager,
			State: "failed"},
		{Name: "PagerDuty", Provider: "pagerduty", Kind: channelPager,
			State: "closed", At: closed},
		{Name: "Slack", Provider: "slack", Kind: channelChat,
			State: "sent", At: paged},
	}, got)
}
