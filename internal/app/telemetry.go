package app

import (
	"context"
	"net/http"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/persistence"
	"github.com/abahmed/kwatch/internal/telemetry"
)

func configureTelemetryRunner(
	telemetryConfig config.Telemetry,
	persistenceManager persistence.TelemetryStore,
	clusterID, version string,
	now func() time.Time,
	client *http.Client,
) func(context.Context) error {
	if !telemetryConfig.Enabled || version == "dev" ||
		os.Getenv("CI") != "" ||
		persistenceManager == nil || clusterID == "" || version == "" {
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
	return func(ctx context.Context) error {
		ticker := time.NewTicker(telemetry.WeeklyInterval)
		defer ticker.Stop()
		send := func() {
			sentAt := now()
			lastSent, err := persistenceManager.GetTelemetryLastSent(ctx)
			if err != nil {
				klog.V(3).InfoS(
					"failed to load telemetry heartbeat state", "error", err,
				)
				return
			}
			if !telemetry.ShouldSend(lastSent, sentAt) {
				return
			}
			reportCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err = telemetry.Report(
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
			if err := persistenceManager.SetTelemetryLastSent(ctx, sentAt); err != nil {
				klog.V(3).InfoS("failed to persist telemetry heartbeat time", "error", err)
			}
		}

		send()
		for {
			select {
			case <-ticker.C:
				send()
			case <-ctx.Done():
				return nil
			}
		}
	}
}

var _ persistence.TelemetryStore = (*persistence.Manager)(nil)
