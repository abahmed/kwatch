package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/heartbeat"
)

func monitoredComponent(
	deps *serverDeps,
	name string,
	run func(context.Context, *serverDeps) error,
) componentSpec {
	return newMonitoredSpec(deps, name, func(ctx context.Context) error {
		return run(ctx, deps)
	}, true)
}

// monitoredRun supervises an optional runner and publishes its health.
// A nil runner gives a spec the supervisor skips.
func monitoredRun(
	deps *serverDeps,
	name string,
	run func(context.Context) error,
) componentSpec {
	if run == nil {
		return componentSpec{name: name}
	}
	return newMonitoredSpec(deps, name, run, true)
}

// selfReportedRun is monitoredRun for a component that publishes its own
// health status, such as rbac with its permission sweep. Only a failure
// is published for it, so a restart never overwrites what it reported.
func selfReportedRun(
	deps *serverDeps,
	name string,
	run func(context.Context) error,
) componentSpec {
	if run == nil {
		return componentSpec{name: name}
	}
	return newMonitoredSpec(deps, name, run, false)
}

// cleanStopComponents may return before shutdown without being restarted:
// heartbeat when it is disabled, upgrader when its checks are off.
var cleanStopComponents = map[string]bool{
	"heartbeat": true, "upgrader": true,
}

func newMonitoredSpec(
	deps *serverDeps,
	name string,
	run func(context.Context) error,
	reportStatus bool,
) componentSpec {
	progress := newComponentProgress(componentStartTime(deps))
	if reportStatus {
		run = reportedRun(deps, name, run)
	}
	return componentSpec{
		name:      name,
		cleanStop: cleanStopComponents[name],
		onError:   degrade(deps, name),
		progress:  progress,
		run: func(ctx context.Context) error {
			return runWithProgress(ctx, deps, progress, run)
		},
	}
}

// reportedRun publishes a component as running only once it actually
// runs, and forgets it when it returns cleanly before shutdown (disabled
// or done), so /health never shows a stopped component as running. The
// supervisor publishes failures, including an unexpected clean return.
func reportedRun(
	deps *serverDeps, name string, run func(context.Context) error,
) func(context.Context) error {
	return func(ctx context.Context) error {
		recoverComponent(deps, name)()
		err := run(ctx)
		if err == nil && ctx.Err() == nil && deps.healthServer != nil {
			deps.healthServer.ClearComponentStatus(name)
		}
		return err
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
	// Delivery is ready only once its workers accept jobs.
	if deps.readiness != nil {
		deps.readiness.setCurrent("delivery", true)
	}
	if !deps.deliveryManager.HasProviders() {
		<-ctx.Done()
		return nil
	}
	published := map[string]bool{}
	publishProviderHealth(deps, published)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-deps.deliveryManager.ProviderHealthEvents():
			publishProviderHealth(deps, published)
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

// providerComponentPrefix names each provider's /health component.
const providerComponentPrefix = "provider-"

// publishProviderHealth shows each configured provider's last delivery
// outcome on /health as component provider-<name>. A failing provider
// is degraded with a bounded reason, but never unready: kwatch keeps
// watching and keeps the provider's queue until it recovers. Providers
// removed by a reconfiguration are cleared. published remembers the
// components shown so far.
func publishProviderHealth(deps *serverDeps, published map[string]bool) {
	if deps.healthServer == nil {
		return
	}
	current := map[string]bool{}
	for _, p := range deps.deliveryManager.ProviderHealth() {
		name := providerComponentPrefix + p.Name
		current[name] = true
		if p.Reason == "" {
			deps.healthServer.SetComponentStatus(name, "running", "", true)
			continue
		}
		deps.healthServer.SetComponentStatus(name, "degraded", p.Reason,
			false)
	}
	for name := range published {
		if !current[name] {
			deps.healthServer.ClearComponentStatus(name)
		}
	}
	for name := range published {
		delete(published, name)
	}
	for name := range current {
		published[name] = true
	}
}

func runHeartbeat(ctx context.Context, deps *serverDeps) error {
	return deps.heartbeat.Start(ctx, heartbeatStatus(deps))
}

// heartbeatStatus publishes each ping outcome: a failed ping degrades the
// heartbeat component, the next accepted ping clears it. Heartbeat is
// optional, so neither changes readiness.
func heartbeatStatus(deps *serverDeps) heartbeat.StatusFunc {
	return func(err error) {
		if deps.healthServer == nil {
			return
		}
		if err != nil {
			deps.healthServer.SetComponentStatus(
				"heartbeat", "degraded", "heartbeat_failed", false)
			return
		}
		deps.healthServer.SetComponentStatus("heartbeat", "running", "", true)
	}
}

func runCRDWatcher(ctx context.Context, deps *serverDeps) error {
	return startCRDWatcher(ctx, deps)
}

// aliveInterval is how often the session stamps that monitoring is
// running, so the next start can report how long it was down.
const aliveInterval = time.Minute

func aliveRecorder(
	session *startupManager,
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
