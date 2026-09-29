package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/startup"
)

func monitoredComponent(
	deps *serverDeps,
	name string,
	run func(context.Context, *serverDeps) error,
) componentSpec {
	progress := newComponentProgress(componentStartTime(deps))
	return componentSpec{
		name:      name,
		cleanStop: name == "heartbeat",
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

func runHeartbeat(ctx context.Context, deps *serverDeps) error {
	return deps.heartbeat.Start(ctx)
}

func runCRDWatcher(ctx context.Context, deps *serverDeps) error {
	return startCRDWatcher(ctx, deps)
}

// aliveInterval is how often the session stamps that monitoring is
// running, so the next start can report how long it was down.
const aliveInterval = time.Minute

func aliveRecorder(
	session *startup.StartupManager,
) func(context.Context) error {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(aliveInterval)
		defer ticker.Stop()
		for {
			session.RecordAlive(ctx)
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	}
}
