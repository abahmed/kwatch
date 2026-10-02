package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
)

func newTestReadiness() (*health.HealthServer, *readinessCoordinator) {
	server := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	return server, newReadinessCoordinator(server)
}

func TestReadinessRequiresAllActiveGates(t *testing.T) {
	server, readiness := newTestReadiness()
	readiness.begin(7, true)

	for _, name := range []string{"state", "pipeline", "delivery"} {
		require.False(t, server.Ready(), name)
		readiness.setCurrent(name, true)
	}
	require.True(t, server.Ready())

	readiness.setCurrent("pipeline", false)
	require.False(t, server.Ready())
	readiness.setCurrent("pipeline", true)
	require.True(t, server.Ready())
}

func TestReadinessDeliveryOptionalWhenNotRequired(t *testing.T) {
	server, readiness := newTestReadiness()
	readiness.begin(1, false)
	readiness.setCurrent("state", true)
	readiness.setCurrent("pipeline", true)
	require.True(t, server.Ready())
}

func TestReadinessEndsWithLeadership(t *testing.T) {
	server, readiness := newTestReadiness()
	readiness.begin(3, false)
	readiness.setCurrent("state", true)
	readiness.setCurrent("pipeline", true)
	require.True(t, server.Ready())

	readiness.end(2)
	require.True(t, server.Ready(), "stale epoch must not end readiness")
	readiness.end(3)
	require.False(t, server.Ready())
	readiness.setCurrent("pipeline", true)
	require.False(t, server.Ready(), "no gate reopens after leadership")
}

func TestReadinessWithdrawDropsReadyAndIgnoresLateReports(t *testing.T) {
	coordinator := newReadinessCoordinator(nil)
	coordinator.begin(1, false)
	coordinator.setCurrent("state", true)
	coordinator.setCurrent("pipeline", true)
	require.True(t, coordinator.readyNowLocked())

	coordinator.withdraw()
	coordinator.setCurrent("pipeline", true)

	require.False(t, coordinator.readyNowLocked())
	coordinator.begin(2, false)
	require.False(t, coordinator.readyNowLocked(), "a new session starts unready")
}
