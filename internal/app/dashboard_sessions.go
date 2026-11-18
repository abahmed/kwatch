package app

import (
	"github.com/abahmed/kwatch/internal/dashboard"
)

// stateDashboardSessions keeps the dashboard's sessions: a random
// seed, a slow digest of the sign-in secrets and the SHA-256 of each
// session ID with its expiry (dashboard.StoredSessions).
const stateDashboardSessions = "dashboard.sessions"

// dashboardSessionStore keeps the dashboard's sessions in the state
// file.
type dashboardSessionStore struct{ disk diskState }

// LoadSessions implements dashboard.SessionStore.
func (d dashboardSessionStore) LoadSessions() (
	dashboard.StoredSessions, bool, error,
) {
	var state dashboard.StoredSessions
	found, err := d.disk.get(stateDashboardSessions, &state)
	return state, found, err
}

// SaveSessions implements dashboard.SessionStore.
func (d dashboardSessionStore) SaveSessions(
	state dashboard.StoredSessions,
) error {
	return d.disk.put(stateDashboardSessions, state)
}

// attachDashboardSessions keeps the dashboard's sessions in the state
// file while this session holds it, and returns the function that
// takes it back before the file closes. Without a dashboard it does
// nothing.
func attachDashboardSessions(deps *serverDeps, disk diskState) func() {
	d := deps.dashboardServer
	if d == nil || disk.store == nil {
		return func() {}
	}
	d.SetSessionStore(dashboardSessionStore{disk: disk})
	return func() { d.SetSessionStore(nil) }
}

// stateReady marks the state file ready and lends it to the
// dashboard's sessions; the returned function takes it back.
func stateReady(deps *serverDeps, disk diskState) (detach func()) {
	deps.readiness.setCurrent("state", true)
	return attachDashboardSessions(deps, disk)
}
