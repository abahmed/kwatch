package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
)

func TestActiveOptionalComponentsHaveProgressOwnership(t *testing.T) {
	run := func(context.Context) error { return nil }
	deps := &serverDeps{
		clients:   client.ClientSet{Clock: clock.RealClock{}},
		statusRun: run, metricsRun: run, probeRun: run,
		kubeletRun: run, storageRun: run, networkRun: run,
		securityRun: run, controlPlaneRun: run, telemetryRun: run,
		upgradeRun: run,
	}

	for _, component := range activeOptionalComponents(deps) {
		require.NotNil(t, component.run, component.name)
		require.NotNil(t, component.progress, component.name)
	}
}
