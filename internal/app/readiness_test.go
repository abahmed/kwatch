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

	for _, name := range []string{"state", "core", "delivery"} {
		require.False(t, server.Ready(), name)
		readiness.setCurrent(name, true)
	}
	require.True(t, server.Ready())

	readiness.setCurrent("core", false)
	require.False(t, server.Ready())
	readiness.setCurrent("core", true)
	require.True(t, server.Ready())
}

func TestReadinessDeliveryOptionalWhenNotRequired(t *testing.T) {
	server, readiness := newTestReadiness()
	readiness.begin(1, false)
	readiness.setCurrent("state", true)
	readiness.setCurrent("core", true)
	require.True(t, server.Ready())
}

func TestReadinessEndsWithLeadership(t *testing.T) {
	server, readiness := newTestReadiness()
	readiness.begin(3, false)
	readiness.setCurrent("state", true)
	readiness.setCurrent("core", true)
	require.True(t, server.Ready())

	readiness.end(2)
	require.True(t, server.Ready(), "stale epoch must not end readiness")
	readiness.end(3)
	require.False(t, server.Ready())
	readiness.setCurrent("core", true)
	require.False(t, server.Ready(), "no gate reopens after leadership")
}
