package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// stoppedIncident resolved at its still-broken cap; crashing adds the
// note of a container that kept crashing.
func stoppedIncident(id string, crashing bool) incident.Incident {
	p := testIncident(id, "shop", -time.Hour)
	p.State, p.Resolved = incident.Resolved, testEpoch
	reason := incident.StoppedTrackingPrefix + "1h; coverage check continues"
	if crashing {
		reason += incident.StillCrashingNote
	}
	p.Timeline = []incident.Event{
		{At: testEpoch.Add(-time.Hour), Text: "resolved: healthy for 5m"},
		{At: testEpoch, Text: "resolved: " + reason},
	}
	return p
}

func TestDetailSaysTrackingStoppedAndWhy(t *testing.T) {
	s := newTestServer(stoppedIncident("s", true), stoppedIncident("t",
		false), testIncident("o", "shop", -time.Minute))
	d := detailOf(t, s, "s")
	assert.True(t, d.StoppedTracking)
	assert.True(t, d.StillCrashing)
	d = detailOf(t, s, "t")
	assert.True(t, d.StoppedTracking)
	assert.False(t, d.StillCrashing)
	d = detailOf(t, s, "o")
	assert.False(t, d.StoppedTracking || d.StillCrashing)
}

func TestListFiltersAndCountsStoppedTracking(t *testing.T) {
	healed := testIncident("h", "shop", -time.Hour)
	healed.State, healed.Resolved = incident.Resolved, testEpoch
	healed.Timeline = []incident.Event{{At: testEpoch,
		Text: "resolved: healthy for 5m"}}
	s := newTestServer(stoppedIncident("s", false), healed)
	l := decodeList(t, get(t, s,
		"/api/incidents?status=resolved&stopped=true").Body.String())
	assert.Equal(t, "s", listIDs(l))
	assert.True(t, l.Incidents[0].StoppedTracking)
	assert.Equal(t, map[string]int{"true": 1, "false": 1},
		l.Facets.Stopped)
	l = decodeList(t, get(t, s,
		"/api/incidents?status=resolved&stopped=false").Body.String())
	assert.Equal(t, "h", listIDs(l))
}

func TestListLabelsTheRequestedFailureTypes(t *testing.T) {
	l := decodeList(t, get(t, newTestServer(),
		"/api/incidents?mode=CPUHigh&mode=PLEGSlow").Body.String())
	assert.Equal(t, map[string]string{"CPUHigh": "CPU high",
		"PLEGSlow": "PLEG slow"}, l.ModeLabels)
}

func TestDetailLinksMovedIncidentsStillKept(t *testing.T) {
	p := testIncident("p", "shop", -time.Hour)
	p.Moved = incident.Moved{Into: "gone", From: []string{"a", "lost"}}
	a := testIncident("a", "shop", -2*time.Hour)
	a.Headline = "Pods of app-a crash"
	d := detailOf(t, newTestServer(p, a), "p")
	require.NotNil(t, d.Moved)
	assert.Nil(t, d.Moved.Into)
	assert.Equal(t, []IncidentRef{{ID: "a",
		Title: "Pods of app-a crash"}}, d.Moved.From)

	p.Moved = incident.Moved{Into: "lost"}
	d = detailOf(t, newTestServer(p), "p")
	assert.Nil(t, d.Moved)
}

func TestDetailServesPagerSentAndFailureReasons(t *testing.T) {
	s := newTestServer(testIncident("a", "shop", -time.Hour))
	s.SetDeliveryTrail(func(string) []DeliveryAttempt {
		return []DeliveryAttempt{{At: testEpoch, Channel: "slack",
			Kind: "incident", Result: "failed",
			Reason: "token refused (401)"}}
	})
	s.SetDeliveryDetail(func(id string) DeliveryDetail {
		return DeliveryDetail{
			Pager: []PagerRow{{Provider: "pagerduty", State: "open",
				PagedAt: testEpoch}},
			Sent: []SentMessage{
				{Provider: "slack", At: testEpoch, Text: "slack text"},
				{Provider: "email", At: testEpoch, Text: "mail text",
					Cut: true}},
		}
	})
	d := detailOf(t, s, "a")
	require.Len(t, d.Deliveries, 1)
	assert.Equal(t, "token refused (401)", d.Deliveries[0].Reason)
	require.Len(t, d.Pager, 1)
	assert.Equal(t, "open", d.Pager[0].State)
	assert.NotEmpty(t, d.Pager[0].Name)
	require.Len(t, d.Sent, 2)
	assert.Equal(t, "email", d.Sent[0].Provider)
	assert.True(t, d.Sent[0].Cut)
	assert.Equal(t, "slack text", d.Sent[1].Text)
}

func TestDetailSaysLogsCannotBeRead(t *testing.T) {
	s := newTestServer(testIncident("a", "shop", -time.Hour))
	forbidden := true
	s.SetSelf(func() (health.SelfReport, bool) {
		return health.SelfReport{PodLogsForbidden: forbidden}, true
	})
	assert.True(t, detailOf(t, s, "a").LogsNotReadable)
	forbidden = false
	assert.False(t, detailOf(t, s, "a").LogsNotReadable)
}

func TestDetailNamesChangesMadeJustBeforeRecovery(t *testing.T) {
	p := testIncident("r", "shop", -30*time.Minute)
	p.State, p.Resolved = incident.Resolved, testEpoch
	p.RecoveringSince = testEpoch.Add(-3 * time.Minute)
	for _, m := range []int{5, 8, 10} {
		p.Journal.Changes = append(p.Journal.Changes, inventory.Change{
			Entity: p.Root, At: testEpoch.Add(-time.Duration(m) *
				time.Minute),
			Fields: []inventory.FieldChange{{Path: "spec.image",
				Before: "api:1", After: "api:2"}}})
	}
	d := detailOf(t, newTestServer(p), "r")
	require.Len(t, d.ChangedBeforeRecovery, 2)
	assert.True(t, d.ChangedBeforeRecovery[0].At.Equal(
		testEpoch.Add(-5*time.Minute)))
	assert.True(t, strings.HasPrefix(d.ChangedBeforeRecovery[0].Summary,
		"image"))

	p.State, p.Resolved = incident.Open, time.Time{}
	assert.Empty(t, detailOf(t, newTestServer(p), "r").
		ChangedBeforeRecovery)
}

func TestDetailWhySaysKwatchCannotSee(t *testing.T) {
	p := testIncident("c", "shop", -time.Hour)
	p.Journal.Timers = incident.Timers{At: testEpoch, Held: true}
	d := detailOf(t, newTestServer(p), "c")
	require.NotNil(t, d.Why)
	assert.True(t, d.Why.CannotSee)
}
