package app

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/controller"
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
		{
			name:      "incident-cleanup",
			onError:   degrade(deps, "incident-cleanup"),
			onHealthy: recoverComponent(deps, "incident-cleanup"),
			run: func(componentCtx context.Context) error {
				return runIncidentCleanup(componentCtx, deps)
			},
		},
		{
			name:      "pvc-monitor",
			onError:   degrade(deps, "pvc-monitor"),
			onHealthy: recoverComponent(deps, "pvc-monitor"),
			run: func(componentCtx context.Context) error {
				return runPVCMonitor(componentCtx, deps)
			},
		},
		{
			name:      "heartbeat",
			onError:   degrade(deps, "heartbeat"),
			onHealthy: recoverComponent(deps, "heartbeat"),
			run: func(componentCtx context.Context) error {
				return runHeartbeat(componentCtx, deps)
			},
		},
		{
			name:      "incident-snapshots",
			onError:   degrade(deps, "incident-snapshots"),
			onHealthy: recoverComponent(deps, "incident-snapshots"),
			run: func(componentCtx context.Context) error {
				return runIncidentSnapshots(componentCtx, deps)
			},
		},
	}

	if deps.tlsSweep != nil {
		components = append(components, componentSpec{
			name: "tls-sweep", onError: degrade(deps, "tls-sweep"),
			onHealthy: recoverComponent(deps, "tls-sweep"),
			run: func(componentCtx context.Context) error {
				return runTLSSweep(componentCtx, deps)
			},
		})
	}
	if deps.runtime.Monitors().CRD().Enabled {
		components = append(components, componentSpec{
			name: "crd-watcher", onError: degrade(deps, "crd-watcher"),
			run: func(componentCtx context.Context) error {
				return runCRDWatcher(componentCtx, deps)
			},
		})
	}
	if deps.runtime.Scope().NamespaceSelector() != "" {
		components = append(components, componentSpec{
			name:      "namespace-scope-watcher",
			onError:   degrade(deps, "namespace-scope-watcher"),
			onHealthy: recoverComponent(deps, "namespace-scope-watcher"),
			run: func(componentCtx context.Context) error {
				return runNamespaceScopeWatcher(componentCtx, deps)
			},
		})
	}
	for _, component := range components {
		supervisor.startOptional(ctx, deps.initialized, component)
	}
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
	select {
	case <-ctx.Done():
		return nil
	case <-deps.deliveryManager.Done():
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("delivery workers stopped unexpectedly")
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
