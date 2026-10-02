package app

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/delivery"
)

// Shutdown budget. Kubernetes kills the Pod terminationGracePeriod after
// SIGTERM (deploy/deploy.yaml and the chart default to 60s), so every
// outer wait is at least the sum of the waits nested inside it and the
// whole sequence fits inside the grace period:
//
//	health stop                  healthStopTimeout
//	leader session (background)  backgroundShutdownTimeout
//	  components                 supervisorShutdownTimeout
//	    one component            componentShutdownTimeout
//	      watcher stop           watcherStopTimeout
//	  delivery drain             deliveryDrainTimeout
//	  outbox final write         delivery.OutboxFinalTimeout
//	  thread flush               threadFinalTimeout
//	  session end                sessionEndTimeout
//	delivery stop (standby)      deliveryStopTimeout
//	lease release                leaseReleaseTimeout
//
// The leader session drains delivery itself, before the final thread
// save. Delivery's last outbox write runs after its drain and has its
// own bound, so it is budgeted separately; otherwise a slow write could
// push the session past the point where the Lease release is skipped.
// The later delivery stop only finishes a manager that no session
// drained, such as on a standby replica, so its bound is short.
const (
	terminationGracePeriod = 60 * time.Second
	shutdownMargin         = 2 * time.Second

	watcherStopTimeout        = 5 * time.Second
	componentShutdownTimeout  = watcherStopTimeout + 5*time.Second
	supervisorShutdownTimeout = componentShutdownTimeout + shutdownMargin
	sessionEndTimeout         = 5 * time.Second
	deliveryDrainTimeout      = 15 * time.Second
	activeSessionTimeout      = supervisorShutdownTimeout +
		deliveryDrainTimeout + delivery.OutboxFinalTimeout +
		threadFinalTimeout + sessionEndTimeout + shutdownMargin
	backgroundShutdownTimeout = activeSessionTimeout + shutdownMargin

	healthStopTimeout   = 5 * time.Second
	deliveryStopTimeout = 2 * time.Second
	leaseReleaseTimeout = 5 * time.Second

	totalShutdownBudget = healthStopTimeout + backgroundShutdownTimeout +
		deliveryStopTimeout + leaseReleaseTimeout
)

// boundedShutdownContext creates an application-owned shutdown deadline. It
// ignores active-component cancellation while retaining context values, so
// final cleanup can complete after cancellation without becoming unbounded.
func boundedShutdownContext(
	parent context.Context, timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}
