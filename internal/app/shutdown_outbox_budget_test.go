package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery"
)

// The delivery drain is followed by the outbox's final write, which has
// its own bound. The session budget must hold both, then the thread
// flush and the session end, and still leave the Lease release inside
// the termination grace period.
func TestSessionBudgetIncludesTheOutboxFinalWrite(t *testing.T) {
	require.GreaterOrEqual(t, activeSessionTimeout,
		supervisorShutdownTimeout+deliveryDrainTimeout+
			delivery.OutboxFinalTimeout+threadFinalTimeout+
			sessionEndTimeout)
	require.LessOrEqual(t,
		healthStopTimeout+backgroundShutdownTimeout+
			deliveryStopTimeout+leaseReleaseTimeout,
		terminationGracePeriod-shutdownMargin)
	require.Equal(t, totalShutdownBudget,
		healthStopTimeout+backgroundShutdownTimeout+
			deliveryStopTimeout+leaseReleaseTimeout)
}
