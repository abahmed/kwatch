package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/crdwatch"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/metrics"
)

const (
	backgroundShutdownTimeout = 10 * time.Second
	componentShutdownTimeout  = 10 * time.Second
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
	if deps.initialized == nil {
		ready := make(chan struct{})
		close(ready)
		deps.initialized = ready
	}
	if deps.controllerDone == nil {
		deps.controllerDone = make(chan struct{})
	}
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

func waitForInitialization(
	ctx context.Context,
	initialized <-chan struct{},
) bool {
	select {
	case <-initialized:
		return true
	case <-ctx.Done():
		return false
	}
}

// startCRDWatcher launches the CRD watcher against the cluster rest config.
func startCRDWatcher(ctx context.Context, deps *serverDeps) error {
	resync := deps.runtime.Lifecycle().ResyncInterval()
	w := crdwatch.NewWithClient(
		deps.runtime, deps.clients.Dynamic, k8s.GetNamespace(), resync,
		deps.cancel,
		func(err error) {
			if err != nil {
				klog.ErrorS(err, "CRD watcher degraded",
					"component", "crd-watcher")
			}
		},
		func(status crdwatch.Status) {
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
	)
	if err := w.Start(ctx); err != nil {
		return fmt.Errorf("crd watcher: %w", err)
	}
	defer func() { _ = w.Stop(nil) }()
	<-ctx.Done()
	return nil
}

func crdWatcherReason(reason string) string {
	switch reason {
	case "cache_sync_failed", "source_not_configured",
		"optional_api_unavailable", "watcher_failed":
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
	select {
	case <-sigCh:
		klog.InfoS("shutting down gracefully...")
	case err := <-supervisor.errCh:
		if err != nil {
			klog.ErrorS(err, "controller startup failed, shutting down")
			exitCode = 1
		}
	case <-applicationContext.Done():
		klog.InfoS("shutting down because the application context was canceled")
	}
	deps.cancel()
	stopHealthServer(deps)
	controllerStopped := waitController(deps)

	// Every producer should stop before the final snapshot. Keep a hard bound
	// so a misbehaving dependency cannot prevent the process from terminating.
	backgroundDone := make(chan struct{})
	go func() {
		supervisor.wg.Wait()
		close(backgroundDone)
	}()
	backgroundStopped := false
	select {
	case <-backgroundDone:
		backgroundStopped = true
	case <-time.After(backgroundShutdownTimeout):
		recordShutdownTimeout("background-tasks")
	}
	if controllerStopped && backgroundStopped &&
		deps.persistenceGate.enabled() {
		incidentStopped := waitIncidentSaver(deps)
		baselineStopped := waitPersistenceComponent(
			deps.baselineDone, "baseline-saver",
		)
		changeStopped := waitPersistenceComponent(
			deps.changeDone, "change-history-saver",
		)
		feedbackStopped := waitFeedbackSaver(deps)
		if incidentStopped && baselineStopped && changeStopped && feedbackStopped {
			finalCtx, cancel := context.WithTimeout(
				context.Background(), componentShutdownTimeout,
			)
			saveFinalIncidentSnapshot(finalCtx, deps)
			cancel()
		} else {
			klog.InfoS(
				"skipping final incident snapshot while savers are still running",
			)
		}
	} else {
		klog.InfoS(
			"skipping final incident snapshot while workers are still running",
		)
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), componentShutdownTimeout,
	)
	if err := deps.deliveryManager.Stop(shutdownCtx); err != nil {
		metrics.DefaultRegistry().ShutdownTimeouts.Add(1)
		klog.ErrorS(err, "timed out waiting for delivery manager to drain")
	}
	cancel()
	if deps.closeAudit != nil {
		if err := deps.closeAudit(); err != nil {
			klog.ErrorS(err, "failed to close audit logger")
		}
	}
	deps.cleanup()
	return exitCode
}

func stopHealthServer(deps *serverDeps) {
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), componentShutdownTimeout,
	)
	defer cancel()
	deps.healthServer.SetReady(false)
	if err := deps.healthServer.Stop(shutdownCtx); err != nil {
		recordShutdownTimeout("health-server")
		klog.ErrorS(err, "failed to stop health check server")
	}
}

func waitController(deps *serverDeps) bool {
	if deps.controllerDone == nil {
		return true
	}
	// The controller owns the event workers that can still mutate the
	// incidentEngine. Its Run method observes the canceled context, so waiting
	// here
	// is required before taking the final snapshot.
	select {
	case <-deps.controllerDone:
		return true
	case <-time.After(componentShutdownTimeout):
		recordShutdownTimeout("controller")
		return false
	}
}

func recordShutdownTimeout(component string) {
	metrics.DefaultRegistry().ShutdownTimeouts.Add(1)
	klog.InfoS("timed out waiting for component", "component", component)
}
