package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// The kubelet's "will be restarted" note is a Normal event that is not
// kept, so Warning liveness failures right before an error exit are the
// evidence that the kubelet stopped the container.
func TestLivenessWarningBeforeAnErrorExitIsALivenessKill(t *testing.T) {
	f := killFixture{state: "running", restarts: 4, exit: 1,
		finished: 20 * time.Second, message: livenessMessage,
		noteAge: 25 * time.Second, threshold: 3}
	found := detectKill(t, f)

	require.Len(t, found, 1)
	assert.Equal(t, reasons.LivenessKilled, found[0].Reason)
	assert.NotContains(t, found[0].Summary, "exiting on its own")
	ctx, e := killContext(f)
	assert.False(t, crashedByItself(ctx, e))
}

// An error exit with no liveness failure behind it is the program's own.
func TestErrorExitWithoutLivenessFailureCrashedByItself(t *testing.T) {
	ctx, e := killContext(killFixture{state: "running", restarts: 4,
		exit: 1, finished: 20 * time.Second})

	assert.False(t, livenessKillAtExit(ctx, e))
	assert.True(t, crashedByItself(ctx, e))
}

// A liveness failure from before livenessNoteLead explained an earlier
// run, not the one that just ended.
func TestOldLivenessWarningLeavesTheCrashItsOwn(t *testing.T) {
	ctx, e := killContext(killFixture{state: "running", restarts: 4,
		exit: 1, finished: 20 * time.Second, message: livenessMessage,
		noteAge: 20*time.Second + livenessNoteLead + time.Second})

	assert.False(t, livenessKillAtExit(ctx, e))
	assert.True(t, crashedByItself(ctx, e))
}

// Fewer failures than the probe's failureThreshold cannot have made the
// kubelet stop the container.
func TestLivenessWarningBelowTheThresholdIsNoKill(t *testing.T) {
	ctx, e := killContext(killFixture{state: "running", restarts: 4,
		exit: 1, finished: 20 * time.Second, message: livenessMessage,
		noteAge: 25 * time.Second, threshold: 5})

	assert.False(t, livenessKillAtExit(ctx, e))
	assert.True(t, crashedByItself(ctx, e))
}
