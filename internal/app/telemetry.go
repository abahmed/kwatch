package app

import (
	"context"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/state"
	"github.com/abahmed/kwatch/internal/telemetry"
)

type telemetryState interface {
	GetTelemetryLastSent(context.Context) time.Time
	SetTelemetryLastSent(context.Context, time.Time) error
}

func configureTelemetryRunner(
	cfg *config.Config,
	stateMgr telemetryState,
	clusterID, version string,
	now func() time.Time,
) func(context.Context) {
	if cfg == nil || !cfg.Telemetry.Enabled || version == "dev" ||
		os.Getenv("CI") != "" ||
		stateMgr == nil || clusterID == "" || version == "" {
		return nil
	}
	// Telemetry is on by default, so this line is how an operator finds out
	// it exists. It names what leaves the cluster, where it goes, and the
	// setting that stops it: a feature that phones home should be legible
	// from the logs alone, without reading the docs first.
	klog.InfoS(
		"adoption telemetry is on: sending a cluster UUID and the kwatch "+
			"version once a week, and nothing else; "+
			"set telemetry.enabled=false to stop it",
		"endpoint", telemetry.Endpoint,
		"interval", telemetry.WeeklyInterval,
	)
	return func(ctx context.Context) {
		client := k8s.GetDefaultClient()
		ticker := time.NewTicker(telemetry.WeeklyInterval)
		defer ticker.Stop()
		send := func() {
			sentAt := now()
			if !telemetry.ShouldSend(stateMgr.GetTelemetryLastSent(ctx), sentAt) {
				return
			}
			reportCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := telemetry.Report(
				reportCtx,
				client,
				telemetry.Endpoint,
				clusterID,
				version,
			)
			cancel()
			if err != nil {
				klog.V(3).InfoS("adoption telemetry heartbeat failed", "error", err)
				return
			}
			if err := stateMgr.SetTelemetryLastSent(ctx, sentAt); err != nil {
				klog.V(3).InfoS("failed to persist telemetry heartbeat time", "error", err)
			}
		}

		send()
		for {
			select {
			case <-ticker.C:
				send()
			case <-ctx.Done():
				return
			}
		}
	}
}

var _ telemetryState = (*state.StateManager)(nil)
