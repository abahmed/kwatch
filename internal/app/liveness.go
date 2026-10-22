package app

import (
	"fmt"
	"time"
)

// pipelineLivenessStall is how long the pipeline loop may go without
// progress before /healthz fails. The loop reports progress on every
// iteration and wakes at least every 10s. The supervisor cancels a
// stalled pipeline long before this; liveness only fails for a loop
// that also ignores cancellation, so the kubelet restarts the process.
// It counts from the session start, so a slow startup is not a stall.
const pipelineLivenessStall = 5 * time.Minute

// armLiveness ties /healthz to pipeline progress for this session and
// returns the function that removes the check again.
func armLiveness(deps *serverDeps) func() {
	if deps.healthServer == nil {
		return func() {}
	}
	deps.healthServer.SetLivenessCheck(pipelineLiveness(
		deps.pipelineProgress, deps.clients.Clock.Now))
	return func() { deps.healthServer.SetLivenessCheck(nil) }
}

// pipelineLiveness fails once progress is older than the stall limit.
func pipelineLiveness(
	progress *componentProgress, now func() time.Time,
) func() error {
	return func() error {
		idle := now().Sub(progress.LastProgress())
		if idle > pipelineLivenessStall {
			return fmt.Errorf("pipeline made no progress for %s",
				idle.Round(time.Second))
		}
		return nil
	}
}
