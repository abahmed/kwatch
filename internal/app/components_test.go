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
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

func componentDeps() *serverDeps {
	return &serverDeps{entitlements: feature.ProEntitlements(),
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
}

func TestMonitoredComponentRunsWithoutFakeProgress(t *testing.T) {
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
	require.False(t, monitoredComponent(deps, "crd-watcher",
		func(context.Context, *serverDeps) error { return nil }).cleanStop)
}

// TestOptionalComponentsReportNoProgress guards against a timer that
// touches progress for an optional component: it would keep the
// supervisor's stall check from ever firing, so the spec must carry no
// progress and the supervisor takes its crash-and-return-only path.
func TestOptionalComponentsReportNoProgress(t *testing.T) {
	deps := componentDeps()
	block := func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}
	specs := []componentSpec{
		monitoredComponent(deps, "threads",
			func(ctx context.Context, _ *serverDeps) error {
				return block(ctx)
			}),
		monitoredRun(deps, "telemetry", block),
		selfReportedRun(deps, "rbac", block),
	}
	for _, spec := range specs {
		require.Nil(t, spec.progress, spec.name)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, spec.run(ctx), spec.name)
	}
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
