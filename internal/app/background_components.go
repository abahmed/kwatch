package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/delivery"
)

// startCoreComponents starts the long-running components owned directly by
// the application. The individual functions keep lifecycle ownership clear.
func startCoreComponents(
	ctx context.Context,
	deps *serverDeps,
	supervisor *componentSupervisor,
) {
	supervisor.startOwned(ctx, componentSpec{
		name:     "controller",
		required: true,
		progress: deps.controllerProgress,
		run: func(ctx context.Context) error {
			return runController(ctx, deps)
		},
	})
	components := []componentSpec{
		monitoredComponent(deps, "incident-cleanup", runIncidentCleanup),
		monitoredComponent(deps, "pvc-monitor", runPVCMonitor),
		monitoredComponent(deps, "heartbeat", runHeartbeat),
		monitoredComponent(deps, "incident-snapshots", runIncidentSnapshots),
	}

	if deps.tlsSweep != nil {
		components = append(
			components,
			monitoredComponent(deps, "tls-sweep", runTLSSweep),
		)
	}
	if deps.runtime.Monitors().CRD().Enabled {
		components = append(
			components,
			monitoredComponent(deps, "crd-watcher", runCRDWatcher),
		)
	}
	if deps.runtime.Scope().NamespaceSelector() != "" {
		components = append(
			components,
			monitoredComponent(
				deps, "namespace-scope-watcher", runNamespaceScopeWatcher,
			),
		)
	}
	for _, component := range components {
		supervisor.startOptional(ctx, deps.initialized, component)
	}
}

func monitoredComponent(
	deps *serverDeps,
	name string,
	run func(context.Context, *serverDeps) error,
) componentSpec {
	progress := newComponentProgress(componentStartTime(deps))
	return componentSpec{
		name:      name,
		onError:   degrade(deps, name),
		onHealthy: recoverComponent(deps, name),
		progress:  progress,
		run: func(ctx context.Context) error {
			return runWithProgress(ctx, deps, progress, func(ctx context.Context) error {
				return run(ctx, deps)
			})
		},
	}
}

func componentStartTime(deps *serverDeps) time.Time {
	if deps == nil || deps.clients.Clock == nil {
		return time.Time{}
	}
	return deps.clients.Clock.Now()
}

func degrade(deps *serverDeps, name string) func(error) {
	return func(err error) {
		if deps.healthServer != nil {
			deps.healthServer.SetComponentError(name, err)
		}
	}
}

func recoverComponent(deps *serverDeps, name string) func() {
	return func() {
		if deps.healthServer != nil {
			deps.healthServer.SetComponentStatus(name, "running", "", true)
		}
	}
}

func runIncidentCleanup(ctx context.Context, deps *serverDeps) error {
	deps.incidentEngine.RunCleanup(ctx)
	return nil
}

func runDelivery(ctx context.Context, deps *serverDeps) error {
	if err := deps.deliveryManager.Start(ctx); err != nil {
		return err
	}
	if !deps.deliveryManager.HasProviders() {
		<-ctx.Done()
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-deps.deliveryManager.ReconfigurationEvents():
			err := deps.deliveryManager.WaitForReconfiguration(ctx)
			if err == nil {
				continue
			}
			if !errors.Is(err, delivery.ErrNoReconfiguration) {
				return err
			}
			return fmt.Errorf(
				"delivery reconfiguration completed without result",
			)
		case <-deps.deliveryManager.Done():
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("delivery workers stopped unexpectedly")
		}
	}
}

func runPVCMonitor(ctx context.Context, deps *serverDeps) error {
	if !waitForInitialization(ctx, deps.initialized) {
		return nil
	}
	return deps.pvcMonitor.Start(ctx)
}

func runHeartbeat(ctx context.Context, deps *serverDeps) error {
	return deps.hbMonitor.Start(ctx)
}

func runIncidentSnapshots(ctx context.Context, deps *serverDeps) error {
	const interval = 60 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	if deps.recordAlive != nil && deps.persistenceGate.enabled() {
		deps.recordAlive(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			trySendIncidentSnapshot(
				deps.incidentCh,
				stateSnapshot{
					incidents: deps.incidentEngine.SnapshotPersisted(),
					groups:    deps.incidentEngine.SnapshotGroups(),
					threads:   deps.deliveryManager.SnapshotThreads(),
					engine:    deps.incidentEngine.SnapshotEngineState(),
				},
			)
			if deps.recordAlive != nil && deps.persistenceGate.enabled() {
				deps.recordAlive(ctx)
			}
		}
	}
}

func runTLSSweep(ctx context.Context, deps *serverDeps) error {
	if err := deps.tlsSweep(); err != nil {
		return err
	}
	const interval = 24 * time.Hour
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := deps.tlsSweep(); err != nil {
				return err
			}
		}
	}
}

func runCRDWatcher(ctx context.Context, deps *serverDeps) error {
	return startCRDWatcher(ctx, deps)
}

func runNamespaceScopeWatcher(ctx context.Context, deps *serverDeps) error {
	namespaces, _ := deps.ctl.NamespaceScope()
	controller.WatchNamespaceScope(
		ctx,
		deps.clients.Kubernetes,
		deps.runtime.Scope().NamespaceSelector(),
		namespaces,
		deps.cancel,
	)
	return nil
}

func runController(ctx context.Context, deps *serverDeps) error {
	defer close(deps.controllerDone)
	if deps.controllerProgress != nil {
		deps.controllerProgress.Touch(deps.clients.Clock.Now())
	}
	// Startup delivery belongs to this owned controller lifecycle goroutine.
	// Delivery itself remains queued and non-blocking.
	deps.notifyStartup()
	workers := deps.runtime.Lifecycle().Workers()
	if workers < 1 {
		workers = 1
	}
	runErr := make(chan error, 1)
	go func() {
		runErr <- deps.ctl.Run(ctx, workers)
	}()
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case suppressed := <-deps.ctl.StartupSummaries():
			if deps.notifyStartupSummary != nil {
				deps.notifyStartupSummary(suppressed)
			}
		case err := <-runErr:
			return err
		case now := <-heartbeat.C:
			if deps.controllerProgress != nil {
				deps.controllerProgress.Touch(now)
			}
		case <-ctx.Done():
			return <-runErr
		}
	}
}
