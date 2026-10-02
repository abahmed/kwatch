package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
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
		func(status crd.Status) {
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
		},
		loadConfig,
	)
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
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	applicationContext := deps.ctx
	if applicationContext == nil {
		applicationContext = context.Background()
	}
	exitCode := 0
	var failureErr error
	select {
	case <-sigCh:
		klog.InfoS("shutting down gracefully...")
	case err := <-supervisor.errCh:
		if err != nil {
			failureErr = err
			exitCode = 1
		}
	case <-applicationContext.Done():
		klog.InfoS("shutting down because the application context was canceled")
	}
	if failureErr != nil {
		klog.ErrorS(failureErr, "shutting down after a component failure")
	}
	deps.cancel()
	stopHealthServer(deps)

	// The leader session stops its components and writes its final state
	// before the supervisor finishes; wait for it with a hard bound so a
	// misbehaving dependency cannot prevent the process from terminating.
	// The bound covers the whole nested session budget (shutdown_context.go).
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

	// The leader session already drained delivery (active.go); this
	// stop finishes a manager no session drained, such as on a standby.
	shutdownCtx, cancel := boundedShutdownContext(
		applicationContext, deliveryStopTimeout)
	if err := deps.deliveryManager.Stop(shutdownCtx); err != nil {
		metrics.DefaultRegistry().ShutdownTimeouts.Add(1)
		klog.ErrorS(err, "timed out waiting for delivery manager to drain")
	}
	cancel()
	releaseLeaseAfterShutdown(deps, applicationContext)
	return exitCode
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
