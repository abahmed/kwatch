package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
)

// waitsIncidents also says which incidents kwatch cannot verify and
// how many wait for the scope, as incident.Manager does.
type waitsIncidents struct {
	fakeIncidents
	unverified map[string]string
	waiting    int
}

func (w *waitsIncidents) Unverified(id string, _ time.Time) string {
	return w.unverified[id]
}

func (w *waitsIncidents) ScopeWaiting() int { return w.waiting }

// An incident kwatch cannot verify now says so in its why.
func TestIncidentWhySaysKwatchCannotVerifyIt(t *testing.T) {
	s := newTestServer()
	line := "kwatch can't verify this right now: reading pod keeps failing"
	s.SetIncidents(&waitsIncidents{
		fakeIncidents: fakeIncidents{items: []incident.Incident{
			testIncident("a", "shop", 0), testIncident("b", "shop", 0)}},
		unverified: map[string]string{"a": line}})

	why := detailOf(t, s, "a").Why
	require.NotNil(t, why)
	assert.Equal(t, line, why.UnverifiedReason)
	if b := detailOf(t, s, "b").Why; b != nil {
		assert.Empty(t, b.UnverifiedReason)
	}
}

// kwatch's own page counts the issues waiting for their namespace's
// labels, and leaves the count out at 0.
func TestKwatchStatusCountsScopeWaiting(t *testing.T) {
	s := newTestServer()
	w := &waitsIncidents{}
	s.SetIncidents(w)
	_, body := readKwatch(t, s)
	assert.NotContains(t, body, "scopeWaiting")

	w.waiting = 2
	got, _ := readKwatch(t, s)
	assert.Equal(t, 2, got.ScopeWaiting)
}
