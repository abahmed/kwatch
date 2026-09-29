package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery"
)

func TestFenceOnLeadershipLossCancelsActiveSession(t *testing.T) {
	leaderCtx, loseLeadership := context.WithCancel(context.Background())
	activeCtx, cancelActive := context.WithCancel(context.Background())
	defer cancelActive()
	done := make(chan struct{})

	go func() {
		fenceOnLeadershipLoss(leaderCtx, activeCtx, cancelActive)
		close(done)
	}()
	loseLeadership()

	<-done
	require.Error(t, activeCtx.Err())
}

func TestFenceOnLeadershipLossReturnsWhenSessionEnds(t *testing.T) {
	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	activeCtx, cancelActive := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		fenceOnLeadershipLoss(leaderCtx, activeCtx, func() {})
		close(done)
	}()
	cancelActive()

	<-done
	require.NoError(t, leaderCtx.Err())
}

func TestWaitForSupervisorReturnsOnceComponentsFinish(t *testing.T) {
	supervisor := newComponentSupervisor(time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	supervisor.startOwned(ctx, componentSpec{
		name: "worker", required: true,
		run: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
	})
	cancel()

	waitForSupervisor(supervisor)
}

func TestWaitForActiveSessionReturnsWhenDone(t *testing.T) {
	done := make(chan struct{})
	close(done)

	require.NoError(t, waitForActiveSession(done))
}

func TestDeliveryProgressRequiresProviders(t *testing.T) {
	require.Nil(t, deliveryProgress(nil))
	require.Nil(t, deliveryProgress(&serverDeps{}))
	deps := &serverDeps{deliveryManager: delivery.
		NewManagerWithDependencies(
			delivery.Dependencies{Clock: clock.RealClock{}})}

	require.Nil(t, deliveryProgress(deps), "no providers, no progress")
}

func TestMonitoredRunSkipsMissingRunner(t *testing.T) {
	spec := monitoredRun(componentDeps(), "telemetry", nil)

	require.Nil(t, spec.run)
	require.Equal(t, "telemetry", spec.name)
}

func TestMonitoredRunWrapsRunnerWithHealthHooks(t *testing.T) {
	deps := componentDeps()
	upgrader := monitoredRun(deps, "upgrader",
		func(context.Context) error { return nil })
	other := monitoredRun(deps, "rbac",
		func(context.Context) error { return nil })

	require.NoError(t, upgrader.run(context.Background()))
	require.True(t, upgrader.cleanStop)
	require.False(t, other.cleanStop)
	require.NotNil(t, other.onError)
	require.NotNil(t, other.onHealthy)
}

func TestApplicationContextDefaultsToBackground(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.Equal(t, ctx, applicationContext(&serverDeps{ctx: ctx}))
	require.NotNil(t, applicationContext(&serverDeps{}))
}

func TestCRDWatcherReasonBoundsUnknownReasons(t *testing.T) {
	for _, known := range []string{
		"cache_sync_failed", "source_not_configured",
		"optional_api_unavailable", "watcher_failed",
	} {
		require.Equal(t, known, crdWatcherReason(known))
	}
	require.Equal(t, "watcher_failed", crdWatcherReason("raw error text"))
}

func TestRecordShutdownTimeoutIsSafeToCall(t *testing.T) {
	recordShutdownTimeout("test-component")
}

func TestRunWithProgressToleratesMissingPieces(t *testing.T) {
	ran := false

	require.NoError(t, runWithProgress(context.Background(), nil, nil, nil))
	require.NoError(t, runWithProgress(context.Background(), nil, nil,
		func(context.Context) error {
			ran = true
			return nil
		}))
	require.True(t, ran)
}

func TestStartProgressHeartbeatStopsOnRequest(t *testing.T) {
	stop := startProgressHeartbeat(context.Background(), func() {})

	stop()
	startProgressHeartbeat(context.Background(), nil)()
}

func TestStartProgressHeartbeatStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stop := startProgressHeartbeat(ctx, func() {})

	cancel()

	stop()
}
