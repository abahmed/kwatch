package dashboard

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func readKwatch(t *testing.T, s *Server) (KwatchResponse, string) {
	t.Helper()
	w := get(t, s, "/api/kwatch")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out KwatchResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out, w.Body.String()
}

func TestKwatchStatusWithoutSourcesListsOnlyTheChannels(t *testing.T) {
	s := newTestServer()

	got, body := readKwatch(t, s)

	assert.Nil(t, got.Coverage)
	assert.Nil(t, got.StateFile)
	assert.Nil(t, got.Restarts24h)
	assert.Equal(t, 24, got.DeliveryWindowHours)
	assert.Equal(t, []ChannelDelivery{{Name: "Slack", Provider: "slack",
		Kind: channelChat, LastSuccess: testEpoch}}, got.Delivery)
	assert.NotContains(t, body, "failedSendsLastHour")
}

func TestKwatchStatusTotalsTheCoverage(t *testing.T) {
	s := newTestServer()
	s.SetCoverage(func() kube.Coverage {
		return kube.Coverage{
			Namespaces: kube.NamespaceCoverage{All: true, Watched: 34,
				Excluded: 2},
			Kinds: kube.KindCoverage{Watched: 40, Synced: 37,
				Reasons: map[string]int{kube.ReasonPermissionDenied: 3,
					kube.ReasonSyncPending: 2, kube.ReasonWatchBudget: 5}},
			SyncedAt: testEpoch,
		}
	})

	got, _ := readKwatch(t, s)

	require.NotNil(t, got.Coverage)
	assert.Equal(t, KwatchCoverage{Watched: 40, Synced: 37, Total: 48,
		Unreadable: 3, Unsynced: 2, NotWatched: 5, AllNamespaces: true,
		Namespaces: 34, ExcludedNamespaces: 2, SyncedAt: testEpoch},
		*got.Coverage)
}

func TestKwatchStatusServesTheStateFileAndRestarts(t *testing.T) {
	s := newTestServer()
	s.SetSelf(func() (health.SelfReport, bool) {
		stops := []health.SelfStop{{At: testEpoch}, {At: testEpoch}}
		return health.SelfReport{StateFileBytes: 4096, OutboxPending: 1,
			Restarts: stops}, true
	})
	s.SetFailedSince(func(time.Time) int { return 2 })

	got, _ := readKwatch(t, s)
	require.NotNil(t, got.StateFile)
	assert.Equal(t, StateFileStatus{Bytes: 4096}, *got.StateFile,
		"the cap is unknown without the option")
	assert.Equal(t, 2, *got.Restarts24h)
	assert.Equal(t, 1, *got.OutboxPending)
	assert.Equal(t, 2, *got.FailedSendsLastHour)

	s.opts.StateCapBytes = 512 << 20
	got, _ = readKwatch(t, s)
	assert.Equal(t, StateFileStatus{Bytes: 4096, CapBytes: 512 << 20,
		CapKnown: true}, *got.StateFile)
}

func TestKwatchStatusCountsTheLastDaysSendsPerChannel(t *testing.T) {
	trails := map[string][]DeliveryAttempt{
		"a": {
			{At: testEpoch.Add(-25 * time.Hour), Channel: "slack",
				Result: "ok"},
			{At: testEpoch.Add(-time.Hour), Channel: "slack", Result: "ok"},
			{At: testEpoch.Add(-time.Hour), Channel: "pagerduty",
				Result: "failed", Pager: true},
			{At: testEpoch.Add(-time.Hour), Result: "carried"},
		},
		"b": {
			{At: testEpoch.Add(-time.Minute), Channel: "slack",
				Result: "retrying"},
			{At: testEpoch.Add(-time.Minute), Channel: "email",
				Result: "ok"},
		},
	}
	s := newTestServer(issueAt("a", "x", incident.Notify, time.Hour),
		issueAt("b", "x", incident.Notify, time.Hour))
	s.SetDeliveryTrail(func(id string) []DeliveryAttempt {
		return trails[id]
	})

	got, _ := readKwatch(t, s)

	assert.Equal(t, []ChannelDelivery{
		{Name: "Email", Provider: "email", Kind: channelEmail, Sent: 1},
		{Name: "PagerDuty", Provider: "pagerduty", Kind: channelPager,
			Failed: 1},
		{Name: "Slack", Provider: "slack", Kind: channelChat, Sent: 1,
			Retrying: 1, LastSuccess: testEpoch},
	}, got.Delivery)
}

func TestKwatchStatusNeedsAuthAndGet(t *testing.T) {
	s := newTestServer()

	assert.Equal(t, http.StatusUnauthorized,
		call(t, s, http.MethodGet, "/api/kwatch", "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed,
		call(t, s, http.MethodPost, "/api/kwatch", testToken).Code)
}
