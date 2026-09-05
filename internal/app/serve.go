package app

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/crdwatch"
	"github.com/abahmed/kwatch/internal/k8s"
)

// serve starts the controller loop and background monitors, then waits for
// shutdown, returning the process exit code.
func serve(ctx context.Context, deps *serverDeps) int {
	var wg sync.WaitGroup
	errCh := make(chan error, 1)
	if deps.initialized == nil {
		ready := make(chan struct{})
		close(ready)
		deps.initialized = ready
	}
	if deps.controllerDone == nil {
		deps.controllerDone = make(chan struct{})
	}

	wg.Add(4)
	if deps.tlsSweep != nil {
		wg.Add(1)
	}
	optionalMonitors := []func(context.Context){deps.statusRun, deps.metricsRun, deps.probeRun, deps.kubeletRun, deps.storageRun, deps.networkRun, deps.securityRun, deps.controlPlaneRun, deps.telemetryRun}
	for _, monitor := range optionalMonitors {
		startOptionalMonitor(ctx, &wg, deps.initialized, monitor)
	}

	go func() {
		defer wg.Done()
		deps.correlator.StartCleanup(ctx)
	}()
	go func() {
		defer wg.Done()
		if !waitForInitialization(ctx, deps.initialized) {
			return
		}
		deps.pvcMonitor.Start(ctx)
	}()
	go func() {
		defer wg.Done()
		deps.hbMonitor.Start(ctx)
	}()
	go func() {
		defer wg.Done()
		interval := 60 * time.Second
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// Share the incident-snapshot tick rather than adding another timer:
		// one ConfigMap write a minute is already the cadence here, and it
		// bounds the reported gap to a minute of resolution.
		if deps.recordAlive != nil {
			deps.recordAlive(ctx)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				trySendIncidentSnapshot(
					deps.incidentCh,
					deps.correlator.SnapshotPersisted(),
				)
				if deps.recordAlive != nil {
					deps.recordAlive(ctx)
				}
			}
		}
	}()
	if deps.tlsSweep != nil {
		go func() {
			defer wg.Done()
			deps.tlsSweep()
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					deps.tlsSweep()
				}
			}
		}()
	}
	if deps.cfg.CrdConfig.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			startCRDWatcher(ctx, deps)
		}()
	}

	go func() {
		defer close(deps.controllerDone)
		deps.notifyStartup()

		workers := deps.cfg.Workers
		if workers < 1 {
			workers = 1
		}
		if err := deps.ctl.Run(ctx, workers); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	return waitShutdown(deps, &wg, errCh)
}

func startOptionalMonitor(
	ctx context.Context,
	wg *sync.WaitGroup,
	initialized <-chan struct{},
	monitor func(context.Context),
) {
	if monitor == nil {
		return
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if !waitForInitialization(ctx, initialized) {
			return
		}
		monitor(ctx)
	}()
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
func startCRDWatcher(ctx context.Context, deps *serverDeps) {
	restCfg, err := client.GetRestConfig(&deps.cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to get rest config for CRD watcher")
		return
	}
	resync := time.Duration(deps.cfg.ResyncSeconds) * time.Second
	w := crdwatch.New(deps.cfg, restCfg, k8s.GetNamespace(), resync, deps.cancel)
	if err := w.Start(ctx); err != nil {
		klog.ErrorS(err, "CRD watcher error")
	}
}

// waitShutdown blocks until a signal or controller failure, then drains.
func waitShutdown(
	deps *serverDeps,
	wg *sync.WaitGroup,
	errCh <-chan error,
) int {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case <-sigCh:
		klog.InfoS("shutting down gracefully...")
	case err := <-errCh:
		if err != nil {
			klog.ErrorS(err, "controller startup failed, shutting down")
		}
	}
	deps.cancel()
	controllerStopped := waitController(deps)

	// Every producer should stop before the final snapshot. Keep a hard bound
	// so a misbehaving dependency cannot prevent the process from terminating.
	backgroundDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(backgroundDone)
	}()
	backgroundStopped := false
	select {
	case <-backgroundDone:
		backgroundStopped = true
	case <-time.After(10 * time.Second):
		klog.InfoS("timed out waiting for background tasks")
	}
	if controllerStopped && backgroundStopped {
		incidentStopped := waitIncidentSaver(deps)
		feedbackStopped := waitFeedbackSaver(deps)
		if incidentStopped && feedbackStopped {
			saveFinalIncidentSnapshot(deps)
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

	select {
	case <-deps.alertManager.Done():
	case <-time.After(10 * time.Second):
		klog.InfoS("timed out waiting for alert manager to drain")
	}
	shutdownCtx, sc := context.WithTimeout(context.Background(), 10*time.Second)
	deps.healthServer.SetReady(false)
	if err := deps.healthServer.Stop(shutdownCtx); err != nil {
		klog.ErrorS(err, "failed to stop health check server")
	}
	sc()
	if deps.closeAudit != nil {
		if err := deps.closeAudit(); err != nil {
			klog.ErrorS(err, "failed to close audit logger")
		}
	}
	deps.cleanup()
	return 0
}

func waitController(deps *serverDeps) bool {
	if deps.controllerDone == nil {
		return true
	}
	// The controller owns the event workers that can still mutate the
	// correlator. Its Run method observes the canceled context, so waiting here
	// is required before taking the final snapshot.
	select {
	case <-deps.controllerDone:
		return true
	case <-time.After(10 * time.Second):
		klog.InfoS("timed out waiting for controller")
		return false
	}
}
