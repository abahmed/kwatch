package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// When the fallback takes an opening after a transient primary failure,
// the primary still never heard it: a later resolve to the primary must
// be sent as "opened and resolved", not as a bare resolve.
func TestFallbackTakingOpeningMarksPrimaryOpenLost(t *testing.T) {
	primary := &errorRecorderProvider{name: "Primary",
		err: errors.New("timeout")}
	fb := &errorRecorderProvider{name: "Fallback"}
	m, entry := fallbackPair(primary, fb)

	got := m.deliver(context.Background(), entry, incidentJob("k", "ns"))

	assert.Equal(t, outcomeDelivered, got)
	assert.Equal(t, 1, fb.callCount)
	assert.Equal(t, openLost, m.openFateOf(entry.lookupName(), "k"))
}

// A fallback that takes an update leaves the primary's fate alone.
func TestFallbackTakingUpdateLeavesPrimaryFateAlone(t *testing.T) {
	primary := &errorRecorderProvider{name: "Primary",
		err: errors.New("timeout")}
	fb := &errorRecorderProvider{name: "Fallback"}
	m, entry := fallbackPair(primary, fb)
	job := incidentJob("k", "ns")
	job.incident.Revision = 3

	m.deliver(context.Background(), entry, job)

	assert.Equal(t, openFate(0), m.openFateOf(entry.lookupName(), "k"))
}
