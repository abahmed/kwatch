//go:build e2e

package scenarios

import (
	"context"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/test/e2e/harness"
)

// These are the only waits a scenario should use. Kwatch does not announce
// a failure the moment it happens: a detector first waits until the problem
// has lasted a while (the "sustain" time), then the incident waits a short
// while for related failures to arrive (the "settle" time). When everything
// is healthy again the incident is held open for a while before it is
// closed (the "hold" time).
const (
	// settleTime is how long a new incident waits before it is announced.
	settleTime = incident.DefaultSettle
	// holdTime is how long a healthy incident stays open before it resolves.
	holdTime = incident.DefaultHold
	// slackTime absorbs slow image pulls, pod starts and CI noise.
	slackTime = 2 * time.Minute
)

// startupQuietTime is how long Kwatch collects incidents into its startup
// summary after a cold start (a two minute window in the pipeline).
const startupQuietTime = 150 * time.Second

var startupOnce sync.Once

// waitForColdStart makes the first scenario of a test run wait until Kwatch
// is past its startup window. Later scenarios find it already past.
func waitForColdStart(ctx context.Context, e *harness.Environment) error {
	var err error
	startupOnce.Do(func() { err = e.WaitForKwatchAge(ctx, startupQuietTime) })
	return err
}

// announceWait is how long to wait for an incident to be announced when its
// detector needs the problem to last sustain first. Use 0 for detectors that
// raise at once, such as a crash loop.
func announceWait(sustain time.Duration) time.Duration {
	return sustain + settleTime + slackTime
}

// resolveWait is how long to wait for an announced incident to resolve after
// the problem has been fixed.
func resolveWait() time.Duration {
	return holdTime + settleTime + slackTime
}
