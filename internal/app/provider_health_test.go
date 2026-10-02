package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// rejectingProvider refuses every delivery, like a revoked token.
type rejectingProvider struct{}

func (rejectingProvider) Name() string { return "slack" }

func (rejectingProvider) SendMessage(context.Context, string) error {
	return transport.Permanent(errors.New("invalid_auth"))
}

func (rejectingProvider) SendIncident(
	context.Context, notification.Message,
) error {
	return transport.Permanent(errors.New("invalid_auth"))
}

func managerWithProvider(
	t *testing.T, alert map[string]map[string]interface{},
) *delivery.Manager {
	t.Helper()
	manager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{Clock: clock.RealClock{}})
	require.NoError(t, manager.InitRuntime(
		config.RuntimeConfigFor(&config.Config{Alert: alert}),
		func(string, map[string]interface{},
			transport.ProviderContext) delivery.Provider {
			return rejectingProvider{}
		}))
	return manager
}

func TestProviderHealthIsPublishedWithoutFailingReadiness(t *testing.T) {
	deps := componentDeps()
	deps.healthServer.SetReady(true)
	deps.deliveryManager = managerWithProvider(t,
		map[string]map[string]interface{}{"slack": {}})
	published := map[string]bool{}

	publishProviderHealth(deps, published)
	require.Equal(t, "running",
		deps.healthServer.ComponentStatuses()["provider-slack"].State)

	// Forget the signal configuring the providers raised.
	select {
	case <-deps.deliveryManager.ProviderHealthEvents():
	default:
	}
	require.NoError(t, deps.deliveryManager.Start(context.Background()))
	deps.deliveryManager.Notify("startup")
	select {
	case <-deps.deliveryManager.ProviderHealthEvents():
	case <-time.After(5 * time.Second):
		t.Fatal("no provider health change")
	}
	publishProviderHealth(deps, published)
	status := deps.healthServer.ComponentStatuses()["provider-slack"]
	require.Equal(t, "degraded", status.State)
	require.Equal(t, "provider_rejected", status.Reason)
	require.True(t, deps.healthServer.Ready(),
		"a provider outage never fails readiness")
	stopCtx, cancel := context.WithTimeout(context.Background(),
		5*time.Second)
	defer cancel()
	require.NoError(t, deps.deliveryManager.Stop(stopCtx))

	deps.deliveryManager = managerWithProvider(t,
		map[string]map[string]interface{}{})
	publishProviderHealth(deps, published)
	require.NotContains(t, deps.healthServer.ComponentStatuses(),
		"provider-slack", "a removed provider is cleared")
}
