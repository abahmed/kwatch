package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A tick that panics after an incident resolved does not lose the
// resolve: the incident goes back to where it was and the next tick
// decides it again. Before, it stayed Resolved with no decision ever
// sent, so its announcement was never closed.
func TestPanicDuringResolveIsRetriedNextTick(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	wantAction(t, r.tick(at(DefaultSettle)), Announce, "settled")
	gone := at(100 * time.Second)
	r.clear(gone, web)
	r.tick(gone)
	good := r.m.model
	// A model with no state panics when the resolve reads it.
	r.m.model = &inventory.Model{}

	ds := r.tick(gone.Add(DefaultHold))

	require.Empty(t, ds)
	assert.NotEqual(t, Resolved, r.only().State, "the resolve is not lost")
	r.m.model = good
	wantAction(t, r.tick(gone.Add(DefaultHold+time.Second)), Resolve,
		"healthy for 3m0s")
}

// A tick that panics after it decided an announcement does not leave
// the incident believing it was sent: the announce state goes back and
// the next tick announces it again.
func TestPanicDuringAnnounceIsRetriedNextTick(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.raise(at(0), web)
	good := r.m.model
	r.m.model = &inventory.Model{}

	ds := r.tick(at(DefaultSettle))

	require.Empty(t, ds)
	p := r.only()
	assert.NotEqual(t, Open, p.State, "the announce is not lost")
	assert.True(t, p.Announced.IsZero())
	assert.Zero(t, p.Revision)
	assert.Empty(t, p.decided)
	r.m.model = good
	wantAction(t, r.tick(at(DefaultSettle+time.Second)), Announce,
		"settled")
}
