package app

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/storage"
)

// An edition change rebuilds the application in process (RunWithClock). The
// new generation reopens the state file and rebinds the health port that the
// previous one just released, so the steps that touch them retry briefly on
// the errors a slow release causes, then fail loudly.

const (
	// rebuildBackoffInitial is the first wait; each retry doubles it.
	rebuildBackoffInitial = time.Second
	// rebuildRetryBudget caps the waits of one step. The old health
	// listener answers liveness until the next generation takes the port
	// (generationEnv.run), so only the port-binding retries run without
	// one; the budget keeps them inside the liveness probe's tolerance
	// (45s in deploy/deploy.yaml).
	rebuildRetryBudget = 30 * time.Second
	// storeReleaseWait is how long a rebuild waits for the previous
	// generation's session to close the state file after the shutdown
	// budgets ran out: the session end plus its margin.
	storeReleaseWait = sessionEndTimeout + shutdownMargin
)

// sleepFunc waits d, or returns the context error when ctx ends first.
type sleepFunc func(ctx context.Context, d time.Duration) error

func sleepContext(ctx context.Context, d time.Duration) error {
	if sleepOrDone(ctx, d) {
		return nil
	}
	return ctx.Err()
}

// rebuildReason is why a generation asked for the next one.
type rebuildReason string

const (
	rebuildNone rebuildReason = ""
	// rebuildEdition: the license changed the edition.
	rebuildEdition rebuildReason = "edition"
	// rebuildLeadershipLost: the Lease was lost while the application
	// still ran; the next generation rejoins the election.
	rebuildLeadershipLost rebuildReason = "leadership_lost"
)

// rebuildReason is why this generation asked for a new one, if it did.
// An edition change wins: it also changes what the next one runs.
func (d *serverDeps) rebuildReason() rebuildReason {
	switch {
	case d.editionRestart.Load():
		return rebuildEdition
	case d.leaseLost.Load():
		return rebuildLeadershipLost
	default:
		return rebuildNone
	}
}

// rebuildRetry retries one step of a rebuild on transient errors. The nil
// value runs the step once, which is how the first start behaves.
type rebuildRetry struct {
	sleep sleepFunc
	// reason names the rebuild in logs and metrics; empty is an edition
	// rebuild.
	reason rebuildReason
}

// forReason is a copy of r that reports for reason.
func (r *rebuildRetry) forReason(reason rebuildReason) *rebuildRetry {
	if r == nil {
		return nil
	}
	return &rebuildRetry{sleep: r.sleep, reason: reason}
}

func (r *rebuildRetry) rejoin() bool {
	return r != nil && r.reason == rebuildLeadershipLost
}

// transientRebuildError reports whether err can clear by waiting for the
// previous generation: the state file lock still held, or the health port
// still bound. Anything else is permanent.
func transientRebuildError(err error) bool {
	return storage.IsLockTimeout(err) || errors.Is(err, syscall.EADDRINUSE)
}

// do runs op, retrying transient failures with doubling waits (1s, 2s, 4s
// ...) until the waits add up to rebuildRetryBudget. The waits go through
// the injected sleep and are summed, not measured, so tests need no clock.
func (r *rebuildRetry) do(
	ctx context.Context, stage string, op func() error,
) error {
	if r == nil {
		return op()
	}
	var spent time.Duration
	delay := rebuildBackoffInitial
	for attempt := 1; ; attempt++ {
		err := op()
		if err == nil || !transientRebuildError(err) {
			return err
		}
		wait := min(delay, rebuildRetryBudget-spent)
		if wait <= 0 {
			return fmt.Errorf("%s: still failing after %d attempts: %w",
				stage, attempt, err)
		}
		klog.InfoS(r.what()+" step failed; retrying",
			"component", "app", "stage", stage, "attempt", attempt,
			"retryIn", wait, "error", err.Error())
		if !r.rejoin() {
			metrics.DefaultRegistry().IncEditionRebuild("retry")
		}
		if serr := r.sleep(ctx, wait); serr != nil {
			return fmt.Errorf("%s: retry canceled: %w: %w",
				stage, serr, err)
		}
		spent += wait
		delay *= 2
	}
}

func (r *rebuildRetry) what() string {
	if r.rejoin() {
		return "leadership rejoin"
	}
	return "edition rebuild"
}

// fail reports a rebuild that gave up, so `kubectl logs --previous` of
// the restarted pod says why. It returns err unchanged.
func (r *rebuildRetry) fail(stage string, err error) error {
	klog.ErrorS(err, r.what()+" failed", "component", "app",
		"stage", stage)
	if r.rejoin() {
		metrics.DefaultRegistry().IncLeadershipRejoin("failed")
	} else {
		metrics.DefaultRegistry().IncEditionRebuild("failed")
	}
	return err
}

// succeed counts a rebuilt generation that is about to serve.
func (r *rebuildRetry) succeed() {
	if r.rejoin() {
		metrics.DefaultRegistry().IncLeadershipRejoin("success")
		return
	}
	metrics.DefaultRegistry().IncEditionRebuild("success")
}

func logRebuild(deps *serverDeps, reason rebuildReason) {
	if reason == rebuildLeadershipLost {
		klog.InfoS("lost the state lock Lease; rejoining leader "+
			"election in a new generation", "component", "app",
			"reason", string(reason))
		return
	}
	klog.InfoS(fmt.Sprintf("edition changed to %s, rebuilding components",
		editionLabel(deps.edition.Load().Edition())), "component", "app",
		"reason", string(reason))
}
