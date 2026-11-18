package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/dashboard"
)

func sessionDashboard(t *testing.T, disk diskState) (
	*dashboard.Server, func(),
) {
	t.Helper()
	dash := dashboard.NewServer(dashboard.Options{Token: "t0k3n",
		Password: "pass-word-1234", Edition: "Pro"}, clock.RealClock{})
	deps := &serverDeps{dashboardServer: dash}
	return dash, attachDashboardSessions(deps, disk)
}

func dashboardSignIn(t *testing.T, dash *dashboard.Server) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(`{"password":"pass-word-1234"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	dash.Handler().ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
	for _, c := range w.Result().Cookies() {
		if c.Value != "" {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

func signedIn(dash *dashboard.Server, c *http.Cookie) bool {
	req := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	dash.Handler().ServeHTTP(w, req)
	return w.Code == http.StatusNoContent
}

// A dashboard sign-in outlives a kwatch restart over the same state
// file; a new state file signs everyone out.
func TestDashboardSessionsSurviveARestartInTheStateFile(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	first, detach := sessionDashboard(t, disk)
	c := dashboardSignIn(t, first)
	detach()

	again, _ := sessionDashboard(t, disk)
	require.True(t, signedIn(again, c), "signed out by a restart")

	var stored dashboard.StoredSessions
	found, err := disk.get(stateDashboardSessions, &stored)
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, stored.Sessions, 1)

	fresh, _ := sessionDashboard(t, diskState{store: openTestStore(t)})
	require.False(t, signedIn(fresh, c), "a new state file keeps sessions")

	none, _ := sessionDashboard(t, diskState{})
	require.False(t, signedIn(none, c), "no state file keeps sessions")
}
