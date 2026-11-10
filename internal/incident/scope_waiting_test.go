package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// An announcement waiting for the scope is counted until the scope
// decides it, either way.
func TestScopeWaitingCountsUntilTheScopeDecides(t *testing.T) {
	r, start := announcedRig(t)
	id := r.only().ID
	assert.Zero(t, r.m.ScopeWaiting())

	r.m.RecordScopeWaiting(id)
	assert.Equal(t, 1, r.m.ScopeWaiting())
	r.m.RecordScopeDecided(id)
	assert.Zero(t, r.m.ScopeWaiting(), "out of scope for good")

	r.m.RecordScopeWaiting(id)
	_, ok := r.m.AnnounceUnheard(id, start.Add(time.Minute))
	assert.True(t, ok)
	assert.Zero(t, r.m.ScopeWaiting(), "announced")

	r.m.RecordScopeWaiting(id)
	r.m.RecordScope(id, false)
	assert.Zero(t, r.m.ScopeWaiting(), "a verdict")
}

// A waiting incident that stops failing is no longer counted.
func TestScopeWaitingLeavesOutWhatNoLongerFails(t *testing.T) {
	r, start := announcedRig(t)
	r.m.RecordScopeWaiting(r.only().ID)
	r.clear(start.Add(time.Minute), podSig("web"))
	r.tick(start.Add(time.Minute))

	assert.Zero(t, r.m.ScopeWaiting())
}
