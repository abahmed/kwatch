package app

import (
	"context"
	"errors"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config/crd"
)

// How a leader session that returned ends the election, and how often a
// session that never got going may rejoin.

// sessionStartError marks an error from a step the leader session runs
// before its components start: the state claim, the edition plan and the
// startup bookkeeping.
type sessionStartError struct{ err error }

func (e *sessionStartError) Error() string { return e.err.Error() }
func (e *sessionStartError) Unwrap() error { return e.err }

func startFailure(err error) error {
	return &sessionStartError{err: err}
}

// retryableStart reports whether err is a session start that can succeed
// by trying again: an API server outage, or a Lease that already names
// another replica. Nothing was sent or written yet, so a rejoin is safe.
func retryableStart(err error) bool {
	var start *sessionStartError
	if !errors.As(err, &start) {
		return false
	}
	return crd.IsRetryableAPIError(err) || errors.Is(err, errNotLeaseHolder)
}

// settle ends the election for a session that returned err. A Lease lost
// while the session ran or was still starting is a loss, whatever the
// session returned: finishElection then rejoins. A session start that
// failed on an outage gives up this term and rejoins too. Any other error
// fails the process as before.
func (c *leaderCallbacks) settle(leaderCtx context.Context, err error) {
	if err == nil {
		return
	}
	if c.parent.Err() == nil && leaderCtx.Err() != nil {
		if !c.deps.sessionLive.Load() {
			c.startFailed.Store(true)
		}
		klog.InfoS("leader session ended after the Lease was lost",
			"component", "app", "error", err.Error())
		return
	}
	if c.parent.Err() == nil && retryableStart(err) {
		klog.ErrorS(err, "leader session could not start; rejoining "+
			"leader election", "component", "app")
		c.startFailed.Store(true)
		c.gaveUp.Store(true)
		c.endElection()
		return
	}
	select {
	case c.activeErrors <- err:
	default:
	}
	c.endElection()
}

// endElection stops the elector on this pod's own decision, as opposed to
// the elector returning because renewal failed.
func (c *leaderCallbacks) endElection() {
	c.ended.Store(true)
	c.cancelElection()
}

const (
	// rejoinBackoffInitial is the first wait before a rejoin that follows
	// a session that never started; each further one in a row doubles it.
	rejoinBackoffInitial = time.Second
	rejoinBackoffMax     = 30 * time.Second
)

// rejoinBackoff is the wait before the next rejoin after failed session
// starts in a row; zero when the last session started.
func rejoinBackoff(failed int) time.Duration {
	if failed <= 0 {
		return 0
	}
	wait := rejoinBackoffInitial
	for i := 1; i < failed && wait < rejoinBackoffMax; i++ {
		wait *= 2
	}
	return min(wait, rejoinBackoffMax)
}

// waitBeforeRejoin paces a rejoin after a session that never started.
// The previous generation's listener keeps answering meanwhile and names
// the loss. It returns false when the generation's context ended first.
func (g generationEnv) waitBeforeRejoin(
	ctx context.Context, prev previousGeneration,
) bool {
	if prev.backoff <= 0 || g.retry == nil {
		return true
	}
	klog.InfoS("leader session failed to start; waiting before "+
		"rejoining", "component", "app", "failedStarts",
		prev.failedStarts, "retryIn", prev.backoff)
	if prev.report != nil {
		prev.report("leadership_lost")
	}
	return g.retry.sleep(ctx, prev.backoff) == nil
}

// errStoppedBeforeRejoin ends a rejoin whose backoff a stop cut short.
var errStoppedBeforeRejoin = errors.New("stopped before rejoining")

// nextPrevious is what the generation after res takes over. failed is
// the count of failed session starts in a row before res.
func nextPrevious(res generationResult, failed int) previousGeneration {
	if res.startFailed {
		failed++
	} else {
		failed = 0
	}
	return previousGeneration{reason: res.reason,
		stopHealth: res.stopHealth, abandon: res.abandon,
		report: res.report, failedStarts: failed,
		backoff: rejoinBackoff(failed)}
}

// release frees what the previous generation still holds when the next
// one is not built: the health port and, after an edition rebuild, the
// Lease. Both steps tolerate having run before.
func (p previousGeneration) release() {
	switch {
	case p.abandon != nil:
		p.abandon()
	case p.stopHealth != nil:
		p.stopHealth()
	}
}
