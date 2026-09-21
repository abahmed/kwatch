package app

import (
	"context"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/persistence"
	"github.com/abahmed/kwatch/internal/telemetry"
)

var telemetryRetryDelays = [...]time.Duration{
	time.Minute, 5 * time.Minute, 15 * time.Minute,
	30 * time.Minute, time.Hour,
}

type telemetryRunnerOptions struct {
	endpoint string
	wait     func(context.Context, time.Duration) bool
	jitter   func(time.Duration) time.Duration
}

func configureTelemetryRunner(
	telemetryConfig config.Telemetry,
	persistenceManager persistence.TelemetryStore,
	clusterID, version string,
	now func() time.Time,
	client *http.Client,
	status *adoptionTelemetryStatus,
) func(context.Context) error {
	return configureTelemetryRunnerWithOptions(
		telemetryConfig, persistenceManager, clusterID, version,
		now, client, status, telemetryRunnerOptions{
			endpoint: telemetry.Endpoint,
			wait:     waitTelemetry, jitter: jitterDuration,
		},
	)
}

func configureTelemetryRunnerWithOptions(
	telemetryConfig config.Telemetry,
	persistenceManager persistence.TelemetryStore,
	clusterID, version string,
	now func() time.Time,
	client *http.Client,
	status *adoptionTelemetryStatus,
	options telemetryRunnerOptions,
) func(context.Context) error {
	if options.wait == nil {
		options.wait = waitTelemetry
	}
	if options.endpoint == "" {
		options.endpoint = telemetry.Endpoint
	}
	if options.jitter == nil {
		options.jitter = jitterDuration
	}
	if status == nil {
		status = newAdoptionTelemetryStatus()
	}
	if reason := telemetrySkipReason(
		telemetryConfig, persistenceManager, clusterID, version,
	); reason != "" {
		status.configure(telemetryConfig.Enabled, "skipped", reason)
		klog.InfoS("adoption telemetry skipped", "reason", reason)
		return nil
	}
	status.configure(true, "waiting", "")
	klog.InfoS("adoption telemetry configured",
		"enabled", true, "endpoint", telemetry.Endpoint,
		"interval", telemetry.WeeklyInterval,
		"retryMaximum", telemetryRetryDelays[len(telemetryRetryDelays)-1],
	)
	return func(ctx context.Context) error {
		failure := -1
		for {
			delay, retry := sendTelemetry(
				ctx, persistenceManager, clusterID, version,
				now, client, options.endpoint, status,
			)
			if ctx.Err() != nil {
				return nil
			}
			if !retry {
				failure = -1
			} else {
				if failure < len(telemetryRetryDelays)-1 {
					failure++
				}
				delay = options.jitter(telemetryRetryDelays[failure])
				metrics.DefaultRegistry().TelemetryRetries.Add(1)
			}
			if !options.wait(ctx, delay) {
				return nil
			}
		}
	}
}

func telemetrySkipReason(
	cfg config.Telemetry,
	store persistence.TelemetryStore,
	clusterID, version string,
) string {
	switch {
	case !cfg.Enabled:
		return "disabled"
	case version == "dev":
		return "dev_build"
	case os.Getenv("CI") != "":
		return "ci_environment"
	case store == nil:
		return "persistence_unavailable"
	case clusterID == "":
		return "missing_cluster_id"
	case version == "":
		return "missing_version"
	default:
		return ""
	}
}

func sendTelemetry(
	ctx context.Context,
	store persistence.TelemetryStore,
	clusterID, version string,
	now func() time.Time,
	client *http.Client,
	endpoint string,
	status *adoptionTelemetryStatus,
) (time.Duration, bool) {
	sentAt := now()
	lastSent, err := store.GetTelemetryLastSent(ctx)
	if err != nil {
		return telemetryFailure(
			status, "state_read", time.Minute, err, now,
		)
	}
	if !telemetry.ShouldSend(lastSent, sentAt) {
		next := lastSent.Add(telemetry.WeeklyInterval)
		if next.Before(sentAt) {
			next = sentAt
		}
		status.waiting(next)
		return next.Sub(sentAt), false
	}

	status.attempt(sentAt)
	metrics.DefaultRegistry().TelemetryAttempts.Add(1)
	reportCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = telemetry.Report(
		reportCtx, client, endpoint, clusterID, version,
	)
	cancel()
	if err != nil {
		reason := telemetryFailureReason(err)
		return telemetryFailure(status, reason, time.Minute, err, now)
	}
	if err := store.SetTelemetryLastSent(ctx, sentAt); err != nil {
		return telemetryFailure(status, "state_write", time.Minute, err, now)
	}
	metrics.DefaultRegistry().TelemetrySuccesses.Add(1)
	next := sentAt.Add(telemetry.WeeklyInterval)
	status.success(sentAt, next)
	klog.InfoS("adoption telemetry sent", "nextAttempt", next)
	return telemetry.WeeklyInterval, false
}

func telemetryFailure(
	status *adoptionTelemetryStatus,
	reason string,
	delay time.Duration,
	err error,
	now func() time.Time,
) (time.Duration, bool) {
	metrics.DefaultRegistry().IncTelemetryFailure(reason)
	next := now().Add(delay)
	status.failure(reason, next)
	klog.ErrorS(err, "adoption telemetry failed", "reason", reason)
	return delay, true
}

func telemetryFailureReason(err error) string {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "invalid telemetry identity") {
		return "invalid_identity"
	}
	if strings.Contains(message, "status ") {
		return "http_status"
	}
	return "network"
}

func jitterDuration(delay time.Duration) time.Duration {
	return delay + time.Duration(rand.Float64()*0.2*float64(delay))
}

func waitTelemetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

var _ persistence.TelemetryStore = (*persistence.Manager)(nil)
