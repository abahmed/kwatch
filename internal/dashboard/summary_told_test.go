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
		{Name: "Webhook", Provider: "webhook", Kind: channelWebhook,
			State: "failed", At: t1},
	}, got)
}

func TestSummaryToldTakesThePagerStateAndPutsPagersFirst(t *testing.T) {
	paged, closed := testEpoch.Add(-time.Hour), testEpoch
	d := IncidentDetail{
		Deliveries: []DeliveryAttempt{
			{At: paged, Channel: "pagerduty", Result: "ok", Keyed: true},
			{At: paged, Channel: "custompager", Result: "ok",
				Keyed: true},
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
		{Name: "Opsgenie", Provider: "opsgenie", Kind: channelPager,
			State: "failed"},
		{Name: "PagerDuty", Provider: "pagerduty", Kind: channelPager,
			State: "closed", At: closed},
		{Name: "Custompager", Provider: "custompager",
			Kind: channelOther, State: "sent", At: paged},
		{Name: "Slack", Provider: "slack", Kind: channelChat,
			State: "sent", At: paged},
	}, got)
}

// A paging service the summary once missed (Datadog, Alerta, Sensu Go,
// New Relic, Splunk HEC) is a pager: the page then says "on-call".
func TestSummaryToldCountsEveryPagingServiceAsAPager(t *testing.T) {
	for _, p := range []string{"datadog", "alerta", "sensugo",
		"newrelic", "splunk"} {
		d := IncidentDetail{DeliveryOutcome: DeliveryOutcome{
			Pager: []PagerRow{{Provider: p, State: "open"}}}}
		got := toldOf(&d)
		assert.Equal(t, channelPager, got[0].Kind, p)
	}
}

// An issue tracker or a webhook keeps one alert by key but pages
// nobody: its open alert is a ticket or a send, never "on-call".
func TestSummaryToldKeyedTrackersAreNotPagers(t *testing.T) {
	at := testEpoch.Add(-time.Hour)
	d := IncidentDetail{
		Deliveries: []DeliveryAttempt{
			{At: at, Channel: "jira", Result: "ok", Keyed: true},
			{At: at, Channel: "github", Result: "ok", Keyed: true},
			{At: at, Channel: "servicenow", Result: "ok", Keyed: true},
			{At: at, Channel: "webhook", Result: "ok", Keyed: true},
		},
		DeliveryOutcome: DeliveryOutcome{Pager: []PagerRow{
			{Provider: "jira", State: "open", PagedAt: at},
			{Provider: "github", State: "open", PagedAt: at},
			{Provider: "servicenow", State: "closed", PagedAt: at},
			{Provider: "webhook", State: "open", PagedAt: at},
		}},
	}
	for _, c := range toldOf(&d) {
		want := channelTicket
		if c.Provider == "webhook" {
			want = channelWebhook
		}
		assert.Equal(t, want, c.Kind, c.Provider)
	}
}
