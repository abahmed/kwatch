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
)

// offServer is a test server whose kwatch was not running for spans.
func offServer(
	spans []health.NotWatchingSpan, items ...incident.Incident,
) *Server {
	s := newTestServer(items...)
	s.SetSelf(func() (health.SelfReport, bool) {
		return health.SelfReport{NotWatching: spans}, true
	})
	return s
}

// A day kwatch was off all along is told apart from a quiet day, and
// a night off is split over the two days it touches.
func TestInsightDaysSayHowLongKwatchWasNotRunning(t *testing.T) {
	day := func(n int) time.Time {
		return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).
			AddDate(0, 0, n)
	}
	s := offServer([]health.NotWatchingSpan{
		{From: day(-4), To: day(-3)},
		{From: day(-2).Add(22 * time.Hour), To: day(-1).Add(6 * time.Hour)},
	})

	w := getPart(t, s, "/api/overview?tzName=UTC", "insights")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got InsightsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))

	require.Len(t, got.Days, trendDays)
	off := map[string][2]int{}
	for _, d := range got.Days {
		off[d.Date] = [2]int{d.OffSeconds, d.Seconds}
	}
	full := int((24 * time.Hour).Seconds())
	assert.Equal(t, [2]int{full, full}, off["2026-09-27"], "off all day")
	assert.Equal(t, [2]int{2 * 3600, full}, off["2026-09-29"])
	assert.Equal(t, [2]int{6 * 3600, full}, off["2026-09-30"])
	assert.Equal(t, [2]int{0, 12 * 3600}, off["2026-10-01"],
		"today counts only the hours so far")
	assert.Equal(t, [2]int{0, full}, off["2026-09-28"])
}

func TestActivityListsWhenKwatchWasNotRunningInTheWindow(t *testing.T) {
	s := offServer([]health.NotWatchingSpan{
		{From: testEpoch.Add(-30 * time.Hour), To: testEpoch.Add(-20 * time.Hour)},
		{From: testEpoch.Add(-2 * time.Hour), To: testEpoch.Add(-30 * time.Minute)},
	})

	w := get(t, s, "/api/activity?since=1h")
	var got ActivityResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, []health.NotWatchingSpan{{From: testEpoch.Add(-time.Hour),
		To: testEpoch.Add(-30 * time.Minute)}}, got.NotWatching,
		"cut to the window")

	w = get(t, s, "/api/activity?since=24h")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.NotWatching, 2)
	assert.Equal(t, testEpoch.Add(-24*time.Hour), got.NotWatching[0].From)
}

func TestIssueDetailMarksTheTimeKwatchWasNotRunning(t *testing.T) {
	p := testIncident("a", "shop", -5*time.Hour)
	resolved := testIncident("b", "shop", -10*time.Hour)
	resolved.State = incident.Resolved
	resolved.Resolved = testEpoch.Add(-9 * time.Hour)
	s := offServer([]health.NotWatchingSpan{
		{From: testEpoch.Add(-3 * time.Hour), To: testEpoch.Add(-time.Hour)},
	}, p, resolved)

	var got IncidentDetail
	w := get(t, s, "/api/incidents/a")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.NotWatching, 1)
	assert.Equal(t, testEpoch.Add(-3*time.Hour), got.NotWatching[0].From)

	got = IncidentDetail{}
	w = get(t, s, "/api/incidents/b")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Empty(t, got.NotWatching, "it had resolved before")
}

func TestKwatchStatusCarriesTheHeartbeat(t *testing.T) {
	s := newTestServer()
	got, body := readKwatch(t, s)
	assert.Nil(t, got.Heartbeat)
	assert.NotContains(t, body, "heartbeat")

	s.SetHeartbeat(func() (health.HeartbeatStatus, bool) {
		return health.HeartbeatStatus{State: health.HeartbeatOK,
			IntervalSeconds: 300, LastPing: testEpoch}, true
	})
	got, _ = readKwatch(t, s)
	require.NotNil(t, got.Heartbeat)
	assert.Equal(t, health.HeartbeatOK, got.Heartbeat.State)
	assert.Equal(t, testEpoch, got.Heartbeat.LastPing)
}
