package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/health"
)

func TestReadinessRequiresAllActiveGates(t *testing.T) {
	server := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	readiness := newReadinessCoordinator(server)
	readiness.begin(7, true)

	for _, name := range []string{
		"restore", "controller", "persistence-writers", "incident",
		"delivery",
	} {
		readiness.setCurrent(name, true)
		if name != "delivery" {
			require.False(t, server.Ready(), name)
		}
	}
	require.True(t, server.Ready())

	readiness.setCurrent("persistence-writers", false)
	require.False(t, server.Ready())
	readiness.setCurrent("persistence-writers", true)
	require.True(t, server.Ready())
}

func TestReadinessIgnoresStaleEpoch(t *testing.T) {
	server := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	readiness := newReadinessCoordinator(server)
	readiness.begin(1, false)
	readiness.setForEpoch(1, "restore", true)
	readiness.begin(2, false)
	readiness.setForEpoch(1, "controller", true)
	require.False(t, server.Ready())

	for _, name := range []string{
		"restore", "controller", "persistence-writers", "incident",
	} {
		readiness.setForEpoch(2, name, true)
	}
	require.True(t, server.Ready())
}

func TestReadinessEndsWithLeadership(t *testing.T) {
	server := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	readiness := newReadinessCoordinator(server)
	readiness.begin(3, false)
	for _, name := range []string{
		"restore", "controller", "persistence-writers", "incident",
	} {
		readiness.setCurrent(name, true)
	}
	require.True(t, server.Ready())
	readiness.end(3)
	require.False(t, server.Ready())
}

func TestReadinessUsesRegisteredPersistenceWriters(t *testing.T) {
	server := health.NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{},
	)
	readiness := newReadinessCoordinator(server)
	readiness.begin(4, false)
	readiness.registerRequiredWriter("baseline-saver")
	readiness.registerRequiredWriter("incident-saver")
	for _, name := range []string{
		"restore", "controller", "incident",
	} {
		readiness.setCurrent(name, true)
	}
	readiness.writerStarted("baseline-saver")
	require.False(t, server.Ready())
	readiness.writerStarted("incident-saver")
	require.True(t, server.Ready())
	readiness.writerFailed("incident-saver")
	require.False(t, server.Ready())
}
