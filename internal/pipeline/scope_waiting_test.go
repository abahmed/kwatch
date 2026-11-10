package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// An announcement waiting for a Namespace's labels is counted as
// waiting, unlike one out of scope, until the scope decides.
func TestScopeWaitingIsCountedUntilTheVerdict(t *testing.T) {
	held, pending := true, true
	h := heldHarness(t, &held, &pending)
	h.run(h.now.Add(5*time.Minute), 10*time.Second)
	assert.Equal(t, 1, h.engine.deps.Incidents.ScopeWaiting())

	pending = false
	h.run(h.now.Add(time.Minute), 10*time.Second)

	assert.Zero(t, h.engine.deps.Incidents.ScopeWaiting(),
		"out of scope for good")
}

// An announcement out of scope from the start is not waiting.
func TestOutOfScopeIsNotCountedAsWaiting(t *testing.T) {
	held, pending := true, false
	h := heldHarness(t, &held, &pending)
	h.run(h.now.Add(5*time.Minute), 10*time.Second)

	assert.Len(t, h.engine.deps.Incidents.Unheard(), 1)
	assert.Zero(t, h.engine.deps.Incidents.ScopeWaiting())
}
