package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
)

func TestHeartbeatNotConfiguredIsShownAsOffNotDegraded(t *testing.T) {
	deps := componentDeps()
	deps.runtime = config.RuntimeConfigFor(config.DefaultConfig())
	deps.heartbeat = heartbeat.NewHeartbeatMonitorWithRuntime(
		deps.runtime, nil, nil)

	err := heartbeatComponent(deps).run(context.Background())

	require.NoError(t, err)
	hb, ok := deps.healthServer.Heartbeat()
	require.True(t, ok)
	require.Equal(t, health.HeartbeatNotConfigured, hb.State)
	component := deps.healthServer.ComponentStatuses()["heartbeat"]
	require.Equal(t, "off", component.State)
	require.Equal(t, "heartbeat_not_configured", component.Reason)
	require.True(t, component.Available)
	require.Empty(t, deps.healthServer.ComponentErrors(),
		"an unconfigured heartbeat must not turn /health red")
}

func TestHeartbeatCommunitySaysItIsAProSetting(t *testing.T) {
	deps := componentDeps()
	deps.entitlements = feature.CommunityEntitlements()

	publishHeartbeatEdition(deps)

	hb, _ := deps.healthServer.Heartbeat()
	require.Equal(t, health.HeartbeatProOnly, hb.State)
	require.Equal(t, "heartbeat_pro_only",
		deps.healthServer.ComponentStatuses()["heartbeat"].Reason)
	require.Empty(t, deps.healthServer.ComponentErrors())

	pro := componentDeps()
	publishHeartbeatEdition(pro)
	_, published := pro.healthServer.Heartbeat()
	require.False(t, published, "Pro publishes from the heartbeat itself")
}

func TestNextHeartbeatKeepsTheLastPingAndWhyItFailed(t *testing.T) {
	t0 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	s := nextHeartbeat(health.HeartbeatStatus{}, nil, t0)
	require.Equal(t, health.HeartbeatOK, s.State)
	require.Equal(t, t0, s.LastPing)

	s = nextHeartbeat(s, fmt.Errorf("wrapped: %w",
		heartbeat.RejectedError{Status: 404}), t0.Add(time.Minute))
	require.Equal(t, health.HeartbeatFailing, s.State)
	require.Equal(t, health.HeartbeatRejected, s.Failure)
	require.Equal(t, 404, s.HTTPStatus)
	require.Equal(t, t0, s.LastPing, "the last good ping is kept")

	s = nextHeartbeat(s, heartbeat.ErrPingFailed, t0.Add(2*time.Minute))
	require.Equal(t, health.HeartbeatUnreachable, s.Failure)
	require.Zero(t, s.HTTPStatus)

	s = nextHeartbeat(s, errors.New("other"), t0.Add(3*time.Minute))
	require.Equal(t, health.HeartbeatUnreachable, s.Failure)

	s = nextHeartbeat(s, nil, t0.Add(4*time.Minute))
	require.Equal(t, health.HeartbeatOK, s.State)
	require.Empty(t, s.Failure)
	require.Equal(t, t0.Add(3*time.Minute), s.LastFailure)
}
