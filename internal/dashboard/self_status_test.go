package dashboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
)

func readStatus(t *testing.T, s *Server) (StatusResponse, string) {
	t.Helper()
	w := get(t, s, "/api/status")
	require.Equal(t, 200, w.Code, w.Body.String())
	var out StatusResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out, w.Body.String()
}

func TestStatusServesSelfAndFailedSendsOfTheLastHour(t *testing.T) {
	s := newTestServer()
	var asked time.Time
	s.SetFailedSince(func(since time.Time) int {
		asked = since
		return 3
	})
	s.SetSelf(func() (health.SelfReport, bool) {
		return health.SelfReport{StartedAt: testEpoch,
			Restarts:       []health.SelfStop{},
			OutboxPending:  &two,
			DashboardLinks: health.LinkStatus{Reason: health.LinksPortMismatch},
			Startup: &health.StartupProgress{Kinds: 30, Synced: 12,
				Required: 4, RequiredSynced: 4}}, true
	})
	got, body := readStatus(t, s)
	require.NotNil(t, got.FailedLastHour)
	assert.Equal(t, 3, *got.FailedLastHour)
	assert.True(t, asked.Equal(testEpoch.Add(-time.Hour)))
	require.NotNil(t, got.Self)
	assert.Equal(t, 2, *got.Self.OutboxPending)
	assert.Equal(t, 12, got.Self.Startup.Synced)
	assert.False(t, got.Self.DashboardLinks.On)
	assert.Equal(t, "port_mismatch", got.Self.DashboardLinks.Reason)
	assert.Contains(t, got.Self.DashboardLinks.Text, `"dashboard" port`)
	for _, key := range []string{`"self":{"startedAt"`,
		`"dashboardLinks":{"on":false,"reason":"port_mismatch","text":`,
		`"startup":{"kinds":30`, `"failedLastHour":3`} {
		assert.True(t, strings.Contains(body, key), "lacks %s: %s", key,
			body)
	}
	assert.NotContains(t, body, "scanEverySeconds")
}

func TestStatusLeavesSelfOutWithoutASession(t *testing.T) {
	s := newTestServer()
	s.SetSelf(func() (health.SelfReport, bool) {
		return health.SelfReport{}, false
	})
	got, body := readStatus(t, s)
	assert.Nil(t, got.Self)
	assert.Nil(t, got.FailedLastHour)
	assert.NotContains(t, body, `"self"`)
}

// The dashboard words each reason as /status does, from health's one
// table, and still says links are off for a reason it does not know.
func TestLinksOffTextNamesEveryReason(t *testing.T) {
	for _, reason := range []string{health.LinksNoRoute,
		health.LinksPortMismatch, health.LinksUnusableAddress,
		health.LinksRoutesForbidden} {
		text := linksOffText(health.LinkStatus{Reason: reason})
		phrase := health.LinkReasonText(reason)
		require.NotEmpty(t, phrase, reason)
		assert.Equal(t,
			"Alerts carry no dashboard link: "+phrase+".", text)
	}
	assert.Equal(t, "Alerts carry no dashboard link.",
		linksOffText(health.LinkStatus{Reason: "other"}))
	assert.Empty(t, linksOffText(health.LinkStatus{On: true}))
}

var one, two = 1, 2
