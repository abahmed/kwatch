package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

func componentDeps() *serverDeps {
	return &serverDeps{
		clients: kubeclient.ClientSet{Clock: clock.RealClock{}},
		healthServer: health.NewHealthServerWithClock(
			config.HealthCheck{}, clock.RealClock{}),
	}
}

func TestComponentHealthTransitions(t *testing.T) {
	deps := componentDeps()

	degrade(deps, "x")(errors.New("boom"))
	degraded := deps.healthServer.ComponentStatuses()["x"]
	recoverComponent(deps, "x")()
	running := deps.healthServer.ComponentStatuses()["x"]

	require.False(t, degraded.Available)
	require.Equal(t, "running", running.State)
	require.True(t, running.Available)
}

func TestComponentHealthHelpersTolerateMissingServer(t *testing.T) {
	deps := &serverDeps{}

	degrade(deps, "x")(errors.New("boom"))
	recoverComponent(deps, "x")()

	require.True(t, componentStartTime(deps).IsZero())
	require.True(t, componentStartTime(nil).IsZero())
}

func TestMonitoredComponentRunsWithProgress(t *testing.T) {
	deps := componentDeps()
	ran := make(chan struct{})
	spec := monitoredComponent(deps, "heartbeat",
		func(context.Context, *serverDeps) error {
			close(ran)
			return nil
		})

	err := spec.run(context.Background())

	<-ran
	require.NoError(t, err)
	require.True(t, spec.cleanStop, "heartbeat may stop cleanly")
	require.False(t, spec.progress.LastProgress().IsZero())
	require.False(t, monitoredComponent(deps, "crd-watcher",
		func(context.Context, *serverDeps) error { return nil }).cleanStop)
}

func TestRunDeliveryWithoutProvidersWaitsForCancellation(t *testing.T) {
	deps := componentDeps()
	deps.deliveryManager = delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- runDelivery(ctx, deps) }()
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("runDelivery did not stop after cancellation")
	}
}

func TestAliveRecorderStampsAndStopsOnCancel(t *testing.T) {
	disk := diskState{store: openTestStore(t)}
	session := newStartupManagerWithRuntime(disk,
		runtimeWith(&config.Config{}), clock.RealClock{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := aliveRecorder(session)(ctx)

	require.NoError(t, err)
	seen, _ := disk.GetLastSeen(context.Background())
	require.False(t, seen.IsZero(), "liveness must be stamped first")
}

func TestHeartbeatStatusDegradesOnFailedPing(t *testing.T) {
	deps := componentDeps()
	report := heartbeatStatus(deps)

	report(errors.New("ping rejected"))
	failed := deps.healthServer.ComponentStatuses()["heartbeat"]
	_, listed := deps.healthServer.ComponentErrors()["heartbeat"]
	report(nil)
	recovered := deps.healthServer.ComponentStatuses()["heartbeat"]

	require.Equal(t, "heartbeat_failed", failed.Reason)
	require.True(t, listed, "a failed ping is listed as degraded")
	require.Equal(t, "running", recovered.State)
	require.NotContains(t, deps.healthServer.ComponentErrors(), "heartbeat")
}
