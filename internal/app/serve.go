package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config/crd"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/metrics"
)

// serve starts the controller loop and background monitors, then waits for
// shutdown, returning the process exit code.
func serve(ctx context.Context, deps *serverDeps) int {
	if ctx == nil {
		ctx = context.Background()
	}
	// The context passed to serve is the application lifecycle boundary. Keep
	// it on the dependency bundle so waitShutdown observes parent cancellation
	// even when bootstrap did not provide a separate derived context.
	deps.ctx = ctx
	supervisor := newComponentSupervisor(deps.clients.Clock.Now)
	if deps.runtime.Lifecycle().HealthCheck().Enabled {
		supervisor.startOwned(ctx, componentSpec{
			name:     "health-server",
			required: true,
			run:      deps.healthServer.Serve,
		})
	}
	startDashboard(ctx, deps, supervisor)
	supervisor.startOwned(ctx, componentSpec{
		name:     "leader-election",
		required: true,
		run: func(ctx context.Context) error {
			return runLeaderElection(ctx, deps)
		},
	})

	return waitShutdown(deps, supervisor)
}

// startCRDWatcher launches the CRD watcher against the cluster rest config.
func startCRDWatcher(ctx context.Context, deps *serverDeps) error {
	resync := deps.runtime.Lifecycle().ResyncInterval()
	w := crd.NewWithClient(
		deps.runtime, deps.clients.Dynamic, kubeclient.GetNamespace(), resync,
		deps.cancel,
		func(err error) {
			if err != nil {
				klog.ErrorS(err, "CRD watcher degraded",
					"component", "crd-watcher")
			}
		},
		crdWatcherStatus(deps),
		loadConfig,
	)
	// The overlay was applied at bootstrap; the watcher starts only now.
	// What changed in between restarts kwatch, like a later edit.
	if deps.overlay.listed {
		w.ExpectStartup(deps.overlay.signature)
	}
	if err := w.Start(ctx); err != nil {
		return fmt.Errorf("crd watcher: %w", err)
	}
	defer func() {
		stopCtx, cancel := boundedShutdownContext(ctx, watcherStopTimeout)
		defer cancel()
		if err := w.Stop(stopCtx); err != nil {
			metrics.DefaultRegistry().ShutdownTimeouts.Add(1)
			klog.ErrorS(err, "CRD watcher shutdown timed out",
				"component", "crd-watcher")
		}
	}()
	<-ctx.Done()
	return nil
}

// crdWatcherStatus publishes the watcher's state on /health.
func crdWatcherStatus(deps *serverDeps) func(crd.Status) {
	return func(status crd.Status) {
		if status.State == "waiting" {
			deps.healthServer.SetComponentStatus(
				"crd-watcher", "waiting",
				"optional_api_unavailable", false,
			)
			return
		}
		if status.State == "degraded" {
			deps.healthServer.SetComponentStatus(
				"crd-watcher", "degraded",
				crdWatcherReason(status.LastError), false,
			)
			return
		}
		if status.Ready {
			deps.healthServer.SetComponentStatus(
				"crd-watcher", "running", "", true,
			)
		}
	}
}

func crdWatcherReason(reason string) string {
	switch reason {
	case "cache_sync_failed", "source_not_configured",
		"optional_api_unavailable", "watcher_failed",
		"config_overlay_invalid":
		return reason
	default:
		return "watcher_failed"
	}
}

// waitShutdown blocks until a signal or controller failure, then drains.
func waitShutdown(
	deps *serverDeps,
	supervisor *componentSupervisor,
) int {
	applicationContext := deps.ctx
	if applicationContext == nil {
		applicationContext = context.Background()
	}
	exitCode := 0
	var failureErr error
	stopped := false
	select {
	case <-deps.stop:
		stopped = true
	case err := <-supervisor.errCh:
		failureErr, exitCode = componentExit(err)
	case <-applicationContext.Done():
		stopped = true
	}
	// A stop wins over a rebuild that was asked for at the same moment.
	stopped = stopped || stopRequested(deps.stop) ||
		applicationContext.Err() != nil
	logShutdownCause(deps, stopped, failureErr)
	// Only a rebuild that the ended leader session asked for skips the
	// wait below. After a stop the session may still be draining, even
	// when the license asked for a rebuild just before.
	rebuild := !stopped && deps.rebuildReason() != rebuildNone
	deps.stopped.Store(stopped)
	deps.cancel()
	stopServersAtExit(deps, rebuild)
	waitBackground(supervisor, rebuild)

	// The leader session already drained delivery (active.go); this
	// stop finishes a manager no session drained, such as on a standby.
	shutdownCtx, cancel := boundedShutdownContext(
		applicationContext, deliveryStopTimeout)
	if err := deps.deliveryManager.Stop(shutdownCtx); err != nil {
		metrics.DefaultRegistry().ShutdownTimeouts.Add(1)
		klog.ErrorS(err, "timed out waiting for delivery manager to drain")
	}
	cancel()
	// A rebuild keeps the Lease for the next generation.
	if !rebuild {
		releaseLeaseAfterShutdown(deps, applicationContext)
	}
	return exitCode
}

func logShutdownCause(deps *serverDeps, stopped bool, failure error) {
	switch {
	case failure != nil:
		klog.ErrorS(failure, "shutting down after a component failure")
	case stopRequested(deps.stop):
		klog.InfoS("shutting down gracefully...")
	case stopped:
		klog.InfoS("shutting down because the application context " +
			"was canceled")
	}
}

// waitBackground waits for the leader session to stop its components and
// write its final state, with a hard bound so a misbehaving dependency
// cannot prevent the process from terminating. The bound covers the whole
// nested session budget (shutdown_context.go). On a rebuild the leader
// session has already ended (it is what asked for the rebuild) and the
// health server is deliberately still serving, so there is nothing to
// wait for.
func waitBackground(supervisor *componentSupervisor, rebuild bool) {
	if rebuild {
		return
	}
	backgroundDone := make(chan struct{})
	go func() {
		supervisor.wg.Wait()
		close(backgroundDone)
	}()
	select {
	case <-backgroundDone:
	case <-time.After(backgroundShutdownTimeout):
		recordShutdownTimeout("background-tasks")
	}
}

// stopServersAtExit stops the generation's listeners. An edition rebuild
// keeps answering liveness: a kubelet that finds no listener for its
// whole tolerance restarts the pod, and the rebuild's own steps can take
// that long. Readiness is withdrawn; the next generation closes the
// listener just before it binds the port (generationEnv.run). The
// dashboard always stops: the next generation builds its own for the
// new edition, and an old one left open would keep serving the old
// edition's pages, or hold the port the new one needs.
//
// The two listeners stop at the same time, so together they take one
// healthStopTimeout, which is what the shutdown budget counts
// (shutdown_context.go).
func stopServersAtExit(deps *serverDeps, rebuild bool) {
	if rebuild {
		deps.healthServer.SetReady(false)
		stopDashboardServer(deps)
		return
	}
	var stops sync.WaitGroup
	stops.Go(func() { stopHealthServer(deps) })
	stops.Go(func() { stopDashboardServer(deps) })
	stops.Wait()
}

func stopHealthServer(deps *serverDeps) {
	shutdownCtx, cancel := boundedShutdownContext(
		deps.ctx, healthStopTimeout)
	defer cancel()
	deps.healthServer.SetReady(false)
	if err := deps.healthServer.Stop(shutdownCtx); err != nil {
		recordShutdownTimeout("health-server")
		klog.ErrorS(err, "failed to stop health check server")
	}
}

func stopDashboardServer(deps *serverDeps) {
	if deps.dashboardServer == nil {
		return
	}
	shutdownCtx, cancel := boundedShutdownContext(
		deps.ctx, healthStopTimeout)
	defer cancel()
	if err := deps.dashboardServer.Stop(shutdownCtx); err != nil {
		recordShutdownTimeout("dashboard-server")
		klog.ErrorS(err, "failed to stop dashboard server")
	}
}

func recordShutdownTimeout(component string) {
	metrics.DefaultRegistry().ShutdownTimeouts.Add(1)
	klog.InfoS("timed out waiting for component", "component", component)
}

// releaseLeaseAfterShutdown hands the Lease over only after delivery
// drained, so the next leader never overlaps with this one.
func releaseLeaseAfterShutdown(deps *serverDeps, parent context.Context) {
	release := deps.leaseRelease()
	if release == nil {
		return
	}
	ctx, cancel := boundedShutdownContext(parent, leaseReleaseTimeout)
	defer cancel()
	release(ctx)
}

// componentExit is the failure and exit code for a component's end. The
// license's request for a new generation and a lost Lease are planned
// rebuilds, not failures.
func componentExit(err error) (error, int) {
	switch {
	case err == nil:
		return nil, 0
	case errors.Is(err, errEditionChanged):
		klog.InfoS("rebuilding for the new edition")
		return nil, 0
	case errors.Is(err, errLeaseLostRejoin):
		klog.InfoS("rejoining leader election after losing the Lease")
		return nil, 0
	}
	return err, 1
}
