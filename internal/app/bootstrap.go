package app

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/abahmed/kwatch/internal/alert/catalog"
	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/rbac"
)

// bootstrap is the infrastructure every replica builds before the Lease is
// acquired: clients, health, delivery. Nothing here writes state; that
// belongs to the active session.
type bootstrap struct {
	runtime         config.RuntimeConfig
	clients         client.ClientSet
	healthServer    *health.HealthServer
	deliveryManager *delivery.Manager
	securityMonitor *rbac.Monitor
	heartbeat       *heartbeat.HeartbeatMonitor
	clock           clock.Clock
}

func newBootstrap(
	ctx context.Context,
	cfg *config.Config,
	now func() time.Time,
) (*bootstrap, error) {
	runtime := config.RuntimeConfigFor(cfg)
	clockSource := clock.Func(now)
	clients, err := client.NewClientSetWithRuntime(
		runtime, &net.Resolver{}, clockSource,
	)
	if err != nil {
		return nil, fmt.Errorf("create application clients: %w", err)
	}
	if err := applyStartupCRD(ctx, cfg, clients.Dynamic); err != nil {
		return nil, fmt.Errorf("apply startup CRD configuration: %w", err)
	}
	// The CRD overlay may change monitor and alert settings. Rebuild the
	// immutable snapshot; only the outbound HTTP client depends on it.
	runtime = config.RuntimeConfigFor(cfg)
	clients.HTTP = client.NewHTTPClientWithRuntime(runtime)

	deliveryManager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{HTTPClient: clients.HTTP, Clock: clockSource},
	)
	if err := deliveryManager.InitRuntime(
		runtime, catalog.NewProvider,
	); err != nil {
		return nil, fmt.Errorf("initialize delivery providers: %w", err)
	}
	healthServer := health.NewHealthServerWithClock(
		runtime.Lifecycle().HealthCheck(), clockSource,
	)
	return &bootstrap{
		runtime:         runtime,
		clients:         clients,
		healthServer:    healthServer,
		deliveryManager: deliveryManager,
		securityMonitor: rbac.NewMonitor(clients.Kubernetes,
			rbac.Checks(k8s.GetNamespace(),
				runtime.Lifecycle().CRDEnabled()),
			clockSource, reportPermissions(healthServer)),
		heartbeat: heartbeat.NewHeartbeatMonitorWithRuntime(
			runtime, clients.HTTP),
		clock: clockSource,
	}, nil
}
