package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// A held announcement whose delivery panics loses that one message: the
// panic stops at the held send, the rest still goes, and the incident is
// no longer held, so a restart does not announce it again forever.
func TestHeldSendPanicLosesOnlyThatMessage(t *testing.T) {
	var sent []string
	sink := func(_ context.Context, d incident.Decision,
		_ notification.Message) {
		if d.Incident.ID == "a" {
			panic("delivery bug")
		}
		sent = append(sent, d.Incident.ID)
	}
	e := newTestEngine(t, &fakeClock{now: holdsNow}, sink, nil)
	a := e.announcer
	for _, id := range []string{"a", "b"} {
		a.held = append(a.held, heldAnnouncement{
			decision: incident.Decision{Action: incident.Announce,
				Incident: incident.Incident{ID: id}},
			decided: holdsNow, until: holdsNow,
		})
	}

	require.NotPanics(t, func() {
		a.expireHeld(context.Background(), holdsNow)
	})

	assert.Equal(t, []string{"b"}, sent)
	assert.Empty(t, a.held)
}
