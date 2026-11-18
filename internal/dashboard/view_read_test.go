package dashboard

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// cameBack is an issue that opened three times, twice without a
// "failing again" message (silent returns), so the incident's own
// RepeatCount, the message's count, is 0.
func cameBack() incident.Incident {
	p := testIncident("a", "shop", 0)
	p.Occurrences = []time.Time{testEpoch.Add(-2 * time.Hour),
		testEpoch.Add(-time.Hour), testEpoch}
	return p
}

// The list, the Overview's trends and the issue page give one repeat
// count for the same issue, and say which view they read.
func TestEndpointsAgreeOnHowOftenAnIssueCameBack(t *testing.T) {
	s := newTestServer(cameBack())

	list := get(t, s, "/api/incidents?status=active")
	require.Equal(t, http.StatusOK, list.Code)
	items := decodeList(t, list.Body.String()).Incidents
	require.Len(t, items, 1)
	assert.Equal(t, 2, items[0].RepeatCount)

	over, _ := readOverview(t, s)
	require.NotNil(t, over.Trends)
	require.Len(t, over.Trends.Noisiest, 1)
	assert.Equal(t, 3, over.Trends.Noisiest[0].Opened)
	assert.Equal(t, 2, over.Trends.Noisiest[0].RepeatCount)

	assert.Equal(t, 2, detailOf(t, s, "a").RepeatCount)

	detail := get(t, s, "/api/incidents/a")
	rev := list.Header().Get(revisionHeader)
	assert.NotEmpty(t, rev)
	assert.Equal(t, rev, detail.Header().Get(revisionHeader))
	assert.Equal(t, rev, get(t, s, "/api/overview").Header().
		Get(revisionHeader))
}

// "Told at" is the first message about the issue, though it reopened
// since; the latest is the last update.
func TestTimingToldAtIsTheFirstSendAndLastToldTheLatest(t *testing.T) {
	s := newTestServer(cameBack())
	s.SetDeliveryTrail(func(string) []DeliveryAttempt {
		return []DeliveryAttempt{
			{At: testEpoch.Add(-3 * time.Hour), Channel: "email",
				Result: "ok"},
			{At: testEpoch.Add(-2*time.Hour + 30*time.Second),
				Channel: "slack", Result: "ok"},
			{At: testEpoch.Add(40 * time.Second), Channel: "email",
				Result: "ok"},
			{At: testEpoch.Add(time.Minute), Channel: "slack",
				Result: "failed"},
		}
	})

	tm := detailOf(t, s, "a").Timing
	require.NotNil(t, tm)
	assert.Equal(t, testEpoch.Add(-2*time.Hour+30*time.Second), tm.ToldAt)
	assert.Equal(t, "Slack", tm.ToldVia)
	assert.Equal(t, testEpoch.Add(40*time.Second), tm.LastToldAt)
	assert.Equal(t, "Email", tm.LastToldVia)
}

// Before the engine published, the endpoints answer 503 as when it is
// not running.
func TestNoViewYetIsUnavailable(t *testing.T) {
	s := newTestServer()
	s.SetIncidents(noView{})
	for _, path := range []string{"/api/incidents", "/api/overview",
		"/api/incidents/a", "/api/activity"} {
		assert.Equal(t, http.StatusServiceUnavailable,
			get(t, s, path).Code, path)
	}
}

// noView is a reader whose engine has not published yet.
type noView struct{}

func (noView) View() *incident.View { return nil }
