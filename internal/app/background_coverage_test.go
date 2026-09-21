package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
)

func TestBackgroundComponentHelpersReportHealthAndProgress(t *testing.T) {
	if !componentStartTime(nil).IsZero() ||
		!componentStartTime(&serverDeps{}).IsZero() {
		t.Fatal("missing application clock produced a start time")
	}
	deps := testServerDeps()
	degrade(deps, "worker")(errors.New("failed"))
	status := deps.healthServer.ComponentStatuses()["worker"]
	if status.Available {
		t.Fatalf("degraded component status = %+v", status)
	}
	recoverComponent(deps, "worker")()
	status = deps.healthServer.ComponentStatuses()["worker"]
	if !status.Available {
		t.Fatalf("recovered component status = %+v", status)
	}
	component := monitoredComponent(
		deps, "heartbeat", func(context.Context, *serverDeps) error {
			return nil
		},
	)
	if component.name != "heartbeat" || !component.cleanStop ||
		component.progress == nil || component.run == nil {
		t.Fatal("monitored component was not fully configured")
	}
	progress := newComponentProgress(time.Now())
	if err := runWithProgress(
		context.Background(), deps, progress, nil,
	); err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundRunsStopOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deps := &serverDeps{initialized: make(chan struct{})}
	if err := runPVCMonitor(ctx, deps); err != nil {
		t.Fatal(err)
	}
	if err := runIncidentSnapshots(ctx, &serverDeps{
		persistenceGate: newPersistenceGate(),
	}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := runTLSSweep(ctx, &serverDeps{
		tlsSweep: func() error { calls++; return nil },
	}); err != nil || calls != 1 {
		t.Fatalf("TLS sweep = %v, calls=%d", err, calls)
	}
	if err := runTLSSweep(ctx, &serverDeps{
		tlsSweep: func() error { return errors.New("tls failed") },
	}); err == nil {
		t.Fatal("TLS sweep error was swallowed")
	}
	if got := activeProbesEnabled(config.ActiveProbeMonitor{}); got {
		t.Fatal("empty active probe config was enabled")
	}
}

func TestProgressHeartbeatHandlesNilAndCancellation(t *testing.T) {
	stop := startProgressHeartbeat(context.Background(), nil)
	stop()
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	stop = startProgressHeartbeat(ctx, func() { calls++ })
	cancel()
	stop()
	if calls < 0 {
		t.Fatal("unreachable progress count")
	}
}
