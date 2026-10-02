package app

import (
	"context"
	"errors"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery"
)

// leaseDrainMargin is kept between the end of the delivery drain and the
// moment the Lease can no longer be renewed.
const leaseDrainMargin = 3 * time.Second

// drainBudget bounds the session-end delivery drain by the time left on
// the Lease. lastRenewal is the last successful Lease write; a zero value
// means unknown and leaves the default budget. Another replica can take
// over once the Lease duration has passed since that write, so that is the
// deadline; the renew deadline only governs this replica's own retries. A
// drain that would outlive the Lease gets less time or none, so the rest
// is dead-lettered before another replica can take over.
func drainBudget(
	lastRenewal, now time.Time, limit time.Duration,
) time.Duration {
	if lastRenewal.IsZero() {
		return limit
	}
	left := lastRenewal.Add(leaderLeaseDuration).Sub(now) - leaseDrainMargin
	if left < 0 {
		return 0
	}
	return min(limit, left)
}

// leaseDrainBudget reads the renewal time from the leadership status.
func (s activeShutdown) leaseDrainBudget() time.Duration {
	if s.lastRenewal == nil || s.now == nil {
		return deliveryDrainTimeout
	}
	return drainBudget(s.lastRenewal(), s.now(), deliveryDrainTimeout)
}

// deliveryStopper stops delivery, sending queued jobs while ctx allows.
type deliveryStopper interface {
	Stop(ctx context.Context) error
}

// stopDelivery drains delivery within budget (at most
// deliveryDrainTimeout, less when the Lease is about to expire). After
// leadership is lost it sends nothing: Stop gets an already ended context,
// so every queued job is dead-lettered instead; the outbox keeps them for
// the next leader. It reports false when the final outbox write is still
// running, so the caller must not close the store.
func stopDelivery(
	parent context.Context, stopper deliveryStopper, fenced bool,
	budget time.Duration,
) bool {
	if stopper == nil {
		return true
	}
	ctx, cancel := boundedShutdownContext(parent, budget)
	defer cancel()
	if fenced || budget <= 0 {
		cancel()
	}
	err := stopper.Stop(ctx)
	switch {
	case fenced:
		klog.InfoS("delivery fenced after leadership loss",
			"component", "delivery", "operation", "fence")
	case budget <= 0:
		klog.InfoS("delivery drain skipped: Lease about to expire",
			"component", "delivery", "operation", "fence")
	case err != nil:
		recordShutdownTimeout("delivery")
		klog.ErrorS(err, "timed out waiting for delivery to drain",
			"component", "delivery")
	}
	return !errors.Is(err, delivery.ErrOutboxBusy)
}
