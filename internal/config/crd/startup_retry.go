package crd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilnet "k8s.io/apimachinery/pkg/util/net"

	"github.com/abahmed/kwatch/internal/clock"
)

// The startup KwatchConfig List is the one API call a generation needs
// before it can serve. While the API server is down or overloaded it is
// retried instead of failing the start.
const (
	// startupAttemptTimeout bounds one List: the REST config sets no
	// request timeout, so a blackholed API would otherwise hang it.
	startupAttemptTimeout = 15 * time.Second
	// startupBackoffInitial is the first wait; each retry doubles it up
	// to startupBackoffMax.
	startupBackoffInitial = time.Second
	startupBackoffMax     = 30 * time.Second
)

// StartupRetry paces the retries of the startup List. The nil value runs
// one attempt.
type StartupRetry struct {
	// Budget caps the time spent retrying: no new attempt starts after
	// it. Zero retries until ctx ends.
	Budget time.Duration
	// Now reads the clock for Budget; nil uses the real clock.
	Now func() time.Time
	// Sleep waits d or returns ctx's error; nil uses a timer.
	Sleep func(ctx context.Context, d time.Duration) error
	// OnRetry is told about each failed attempt before its wait.
	OnRetry func(attempt int, wait time.Duration, err error)
	// AuthWindow also retries Unauthorized and Forbidden while the time
	// since the first attempt stays within it. A rebuild sets it: an
	// authentication or authorization webhook can still be down while the
	// API server comes back. Zero (a first start) fails on them at once.
	AuthWindow time.Duration
	// NotFoundWindow retries a NotFound (the resource type is not
	// served) while the time since the first attempt stays within it,
	// then accepts it. Right after an API outage the type can be missing
	// for a moment before its handler is ready. Zero accepts at once.
	NotFoundWindow time.Duration
}

// Do runs op until it succeeds, fails with an error that waiting cannot
// fix, the budget runs out or ctx ends. Each attempt has its own timeout.
func (r *StartupRetry) Do(
	ctx context.Context, op func(context.Context) error,
) error {
	if r == nil {
		return attemptOnce(ctx, op)
	}
	now := r.Now
	if now == nil {
		now = clock.RealClock{}.Now
	}
	start := now()
	delay := startupBackoffInitial
	for attempt := 1; ; attempt++ {
		err := attemptOnce(ctx, op)
		if err == nil || ctx.Err() != nil ||
			!r.retryable(err, now().Sub(start)+delay) {
			return err
		}
		if r.Budget > 0 && now().Sub(start)+delay > r.Budget {
			return fmt.Errorf("API server still unreachable after "+
				"%d attempts: %w", attempt, err)
		}
		if r.OnRetry != nil {
			r.OnRetry(attempt, delay, err)
		}
		if serr := r.sleep(ctx, delay); serr != nil {
			return fmt.Errorf("API retry canceled: %w: %w", serr, err)
		}
		delay = min(delay*2, startupBackoffMax)
	}
}

// retryable reports whether err is worth another attempt that would start
// after elapsed since the first one.
func (r *StartupRetry) retryable(err error, elapsed time.Duration) bool {
	switch {
	case IsRetryableAPIError(err):
		return true
	case apierrors.IsUnauthorized(err) || apierrors.IsForbidden(err):
		return elapsed <= r.AuthWindow
	case apierrors.IsNotFound(err):
		return elapsed <= r.NotFoundWindow
	}
	return false
}

func (r *StartupRetry) sleep(ctx context.Context, d time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func attemptOnce(ctx context.Context, op func(context.Context) error) error {
	attemptCtx, cancel := context.WithTimeout(ctx, startupAttemptTimeout)
	defer cancel()
	return op(attemptCtx)
}

// IsRetryableAPIError reports whether err is an API server outage that
// can clear by waiting: a 5xx, a 429, a server or client timeout, or a
// refused, reset or cut connection. NotFound, Forbidden and every other
// client error are permanent.
func IsRetryableAPIError(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err) ||
		apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) ||
		apierrors.IsTooManyRequests(err) {
		return true
	}
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		return status.Status().Code >= 500
	}
	if utilnet.IsConnectionRefused(err) || utilnet.IsConnectionReset(err) ||
		utilnet.IsProbableEOF(err) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
