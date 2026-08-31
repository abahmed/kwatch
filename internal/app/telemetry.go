package app

import (
	"context"
	"net/http"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
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
	return func(ctx context.Context) {
		client := &http.Client{Timeout: 2 * time.Second}
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
				klog.V(3).InfoS("anonymous telemetry heartbeat failed", "error", err)
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
