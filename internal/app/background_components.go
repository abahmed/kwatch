package app

import (
	"context"
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
	components := []componentSpec{
		{
			name:    "incident-cleanup",
			onError: degrade(deps, "incident-cleanup"),
			run: func(componentCtx context.Context) error {
				return runIncidentCleanup(componentCtx, deps)
			},
		},
		{
			name:    "pvc-monitor",
			onError: degrade(deps, "pvc-monitor"),
			run: func(componentCtx context.Context) error {
				return runPVCMonitor(componentCtx, deps)
			},
		},
		{
			name:    "heartbeat",
			onError: degrade(deps, "heartbeat"),
			run: func(componentCtx context.Context) error {
				return runHeartbeat(componentCtx, deps)
			},
		},
		{
			name:    "incident-snapshots",
			onError: degrade(deps, "incident-snapshots"),
			run: func(componentCtx context.Context) error {
				return runIncidentSnapshots(componentCtx, deps)
			},
		},
	}

	if deps.tlsSweep != nil {
		components = append(components, componentSpec{
			name: "tls-sweep", onError: degrade(deps, "tls-sweep"),
			run: func(componentCtx context.Context) error {
				return runTLSSweep(componentCtx, deps)
			},
		})
	}
	if deps.runtime.CrdConfig().Enabled {
		components = append(components, componentSpec{
			name: "crd-watcher", onError: degrade(deps, "crd-watcher"),
			run: func(componentCtx context.Context) error {
				return runCRDWatcher(componentCtx, deps)
			},
		})
	}
	if deps.runtime.NamespaceSelector() != "" {
		components = append(components, componentSpec{
			name:    "namespace-scope-watcher",
			onError: degrade(deps, "namespace-scope-watcher"),
			run: func(componentCtx context.Context) error {
				return runNamespaceScopeWatcher(componentCtx, deps)
			},
		})
	}
	for _, component := range components {
		supervisor.startOwned(ctx, component)
	}
	supervisor.startOwned(ctx, componentSpec{
		name:     "controller",
		required: true,
		run: func(ctx context.Context) error {
			return runController(ctx, deps)
		},
	})
}

func degrade(deps *serverDeps, name string) func(error) {
	return func(err error) {
		if deps.healthServer != nil {
			deps.healthServer.SetComponentError(name, err)
		}
	}
}

func runIncidentCleanup(ctx context.Context, deps *serverDeps) error {
	deps.incidentEngine.StartCleanup(ctx)
	return nil
}

func runDelivery(ctx context.Context, deps *serverDeps) error {
	if err := deps.deliveryManager.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
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
	if deps.recordAlive != nil {
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
			if deps.recordAlive != nil {
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
		deps.runtime.NamespaceSelector(),
		namespaces,
		deps.cancel,
	)
	return nil
}

func runController(ctx context.Context, deps *serverDeps) error {
	defer close(deps.controllerDone)
	// Startup delivery belongs to this owned controller lifecycle goroutine.
	// Delivery itself remains queued and non-blocking.
	deps.notifyStartup()
	workers := deps.runtime.Workers()
	if workers < 1 {
		workers = 1
	}
	runErr := make(chan error, 1)
	go func() {
		runErr <- deps.ctl.Run(ctx, workers)
	}()
	for {
		select {
		case suppressed := <-deps.ctl.StartupSummaries():
			if deps.notifyStartupSummary != nil {
				deps.notifyStartupSummary(suppressed)
			}
		case err := <-runErr:
			return err
		case <-ctx.Done():
			return <-runErr
		}
	}
}
