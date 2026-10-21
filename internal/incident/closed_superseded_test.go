package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A superseded incident resolved because its failures moved to another
// incident, not because they stopped. A roll-up that named it is not
// over while the incident that took them over still fails.
func TestClosedWaitsForTheIncidentThatTookOverTheFailures(t *testing.T) {
	tr := announcedTieRig(t)
	apiPod := tr.api.Entity
	oldID, newID := tr.idOf(tr.a), tr.idOf(tr.b)
	soon := DefaultSettle + DefaultReviseSettle

	tr.rerank(at(soon), tieCause(tr.b, 0.52, apiPod),
		tieCause(tr.a, 0.50, apiPod))
	tr.tick(at(soon))
	require.Equal(t, Resolved, tr.of(tr.a).State)
	require.Equal(t, newID, tr.of(tr.a).SupersededBy)

	assert.False(t, tr.m.Closed([]string{oldID}),
		"its failures live on in the incident that took them over")
}

// Once the incident that took over resolves too, everything is over.
func TestClosedIsTrueWhenTheSuccessorResolvedToo(t *testing.T) {
	tr := announcedTieRig(t)
	apiPod := tr.api.Entity
	oldID := tr.idOf(tr.a)
	soon := DefaultSettle + DefaultReviseSettle
	tr.rerank(at(soon), tieCause(tr.b, 0.52, apiPod),
		tieCause(tr.a, 0.50, apiPod))
	tr.tick(at(soon))

	for _, f := range tr.active {
		tr.clear(at(soon+time.Second), f)
	}
	tr.tick(at(soon + time.Second))
	tr.tick(at(soon + 2*time.Second + DefaultHold))

	assert.True(t, tr.m.Closed([]string{oldID}))
}

// An incident that resolved and came back after its reopen window is a
// new incident that carries the old one's history; the old one is
// forgotten. A roll-up that named the old one is not over while that
// recurrence still fails.
func TestClosedFollowsTheRecurrenceOfAForgottenIncident(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	announced(t, r, web)
	oldID := r.idOf(web.Entity)
	now := at(DefaultSettle + 10*time.Second)
	r.clear(now, web)
	r.tick(now)
	now = now.Add(DefaultHold + time.Second)
	require.Equal(t, Resolve, r.tick(now)[0].Action)
	require.True(t, r.m.Closed([]string{oldID}), "setup: it is over")

	now = now.Add(RepageWindow + time.Hour)
	r.raise(now, web)
	require.NotEqual(t, oldID, r.idOf(web.Entity), "setup: a recurrence")
	require.Equal(t, oldID, r.of(web.Entity).Previous)

	assert.False(t, r.m.Closed([]string{oldID}),
		"the recurrence of a forgotten incident is still failing")

	r.clear(now.Add(time.Second), web)
	r.tick(now.Add(time.Second))
	r.tick(now.Add(2*time.Second + DefaultHold))
	assert.True(t, r.m.Closed([]string{oldID}), "and over when it is")
}
