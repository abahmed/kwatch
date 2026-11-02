package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A restored incident is never handed over to an incident still
// settling: when the settling one recovers before it is announced, the
// "Moved" line would point at a message nobody ever got. The restored
// incident goes on by itself and resolves with a resolve of its own.
func TestHandoverSkipsAHolderThatWasNeverAnnounced(t *testing.T) {
	grace := 10 * time.Minute
	r, svc, noEndpoints := restoredServiceRig(t, grace)
	oldID := r.only().ID
	deploy := entity(kube.KindDeployment, "warehouse")
	r.cause(svc, deploy, "warehouse crash loops")
	// A warning settles for DefaultSettle, well past the grace.
	noEndpoints.Severity = detection.Warning
	r.raise(at(grace-20*time.Second), noEndpoints)

	got := tickEvery(r, grace-20*time.Second, grace)
	r.clear(at(grace+10*time.Second), noEndpoints)
	got = append(got, tickEvery(r, grace+10*time.Second, 3*time.Hour)...)

	var resolve *Decision
	for i, d := range got {
		assert.NotEqual(t, ReasonSuperseded, d.Reason, "%+v", d)
		assert.NotEqual(t, Announce, d.Action, "the holder never spoke")
		if d.Action == Resolve && d.Incident.ID == oldID {
			resolve = &got[i]
		}
	}
	require.NotNil(t, resolve, "the restored incident resolves: %+v", got)
	assert.Empty(t, resolve.Incident.SupersededBy)
	assert.False(t, resolve.Handover)
}

// The restored incident waits for the holder's announcement and is
// handed over right after it, never before.
func TestHandoverWaitsForTheHolderToBeAnnounced(t *testing.T) {
	grace := 10 * time.Minute
	r, svc, noEndpoints := restoredServiceRig(t, grace)
	oldID := r.only().ID
	deploy := entity(kube.KindDeployment, "warehouse")
	r.cause(svc, deploy, "warehouse crash loops")
	r.raise(at(grace-20*time.Second), noEndpoints)

	announced, moved := -1, -1
	got := tickEvery(r, grace-20*time.Second, grace+5*time.Minute)
	for i, d := range got {
		switch {
		case d.Action == Announce && d.Incident.Root == deploy:
			announced = i
		case d.Action == Resolve && d.Incident.ID == oldID:
			assert.Equal(t, ReasonSuperseded, d.Reason)
			moved = i
		}
	}
	require.GreaterOrEqual(t, announced, 0, "%+v", got)
	assert.Greater(t, moved, announced, "moved after the holder spoke")
}
