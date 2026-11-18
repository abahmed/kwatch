package app

import (
	"context"
	"errors"
	"time"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
)

// defaultHeartbeatInterval is the ping interval heartbeat.Start uses
// when none is set.
const defaultHeartbeatInterval = 300

// heartbeatComponent runs the heartbeat. It publishes its own health
// (heartbeat state), so the supervisor wrapper neither marks it
// running nor forgets it when it stops because it is not configured.
func heartbeatComponent(deps *serverDeps) componentSpec {
	return newMonitoredSpec(deps, health.HeartbeatComponent,
		func(ctx context.Context) error {
			return runHeartbeat(ctx, deps)
		}, false)
}

// runHeartbeat pings while configured; without a URL it says so on
// /health and stops.
func runHeartbeat(ctx context.Context, deps *serverDeps) error {
	interval, ok := heartbeatInterval(deps)
	if !ok {
		publishHeartbeat(deps, health.HeartbeatStatus{
			State: health.HeartbeatNotConfigured})
		return nil
	}
	publishHeartbeat(deps, health.HeartbeatStatus{
		State: health.HeartbeatWaiting, IntervalSeconds: interval})
	return deps.heartbeat.Start(ctx, heartbeatStatus(deps))
}

// heartbeatInterval is the configured ping interval in seconds; ok is
// false while the heartbeat is off or has no URL.
func heartbeatInterval(deps *serverDeps) (int, bool) {
	if deps.heartbeat == nil {
		return 0, false
	}
	cfg := deps.runtime.Lifecycle().Heartbeat()
	if !cfg.Enabled || cfg.URL == "" {
		return 0, false
	}
	if cfg.Interval <= 0 {
		return defaultHeartbeatInterval, true
	}
	return cfg.Interval, true
}

// publishHeartbeatEdition says on /health that the Community edition
// has no heartbeat, so its absence is not mistaken for one that works.
func publishHeartbeatEdition(deps *serverDeps) {
	if !deps.has(feature.Heartbeat) {
		publishHeartbeat(deps, health.HeartbeatStatus{
			State: health.HeartbeatProOnly})
	}
}

// publishHeartbeat hands a heartbeat state to the health server.
func publishHeartbeat(deps *serverDeps, status health.HeartbeatStatus) {
	if deps.healthServer != nil {
		deps.healthServer.SetHeartbeat(status)
	}
}

// heartbeatStatus publishes each ping outcome: a failed ping degrades
// the heartbeat component, the next accepted ping clears it. Heartbeat
// is optional, so neither changes readiness. It is called from the
// heartbeat's goroutine only.
func heartbeatStatus(deps *serverDeps) heartbeat.StatusFunc {
	interval, _ := heartbeatInterval(deps)
	status := health.HeartbeatStatus{IntervalSeconds: interval}
	return func(err error) {
		status = nextHeartbeat(status, err, deps.clients.Clock.Now())
		publishHeartbeat(deps, status)
	}
}

// nextHeartbeat is the heartbeat state after a ping at now that ended
// with err (nil: accepted).
func nextHeartbeat(
	status health.HeartbeatStatus, err error, now time.Time,
) health.HeartbeatStatus {
	if err == nil {
		status.State, status.LastPing = health.HeartbeatOK, now
		status.Failure, status.HTTPStatus = "", 0
		return status
	}
	status.State, status.LastFailure = health.HeartbeatFailing, now
	status.Failure, status.HTTPStatus = health.HeartbeatUnreachable, 0
	var rejected heartbeat.RejectedError
	if errors.As(err, &rejected) {
		status.Failure = health.HeartbeatRejected
		status.HTTPStatus = rejected.Status
	} else if errors.Is(err, heartbeat.ErrPingRejected) {
		status.Failure = health.HeartbeatRejected
	}
	return status
}
