package app

import (
	"context"
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/alert/catalog"
	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/persistence"
	"github.com/abahmed/kwatch/internal/rbac"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/upgrader"
)

// bootstrap contains infrastructure and startup state that must exist before
// domain engines or monitor families can be composed.
type bootstrap struct {
	runtime         config.RuntimeConfig
	persistence     *persistence.Manager
	startupManager  *startup.StartupManager
	startupResult   startup.Result
	healthServer    *health.HealthServer
	securityMonitor *rbac.Monitor
	deliveryManager *delivery.Manager
	clients         client.ClientSet
	telemetryRun    func(context.Context)
	upgradeRun      func(context.Context)
}

func newBootstrap(
	ctx context.Context,
	cfg *config.Config,
	now func() time.Time,
) (*bootstrap, error) {
	runtime := config.RuntimeConfigFor(cfg)
	upgraderConfig := runtime.Upgrader()
	clients, err := client.NewClientSetWithRuntime(runtime)
	if err != nil {
		return nil, fmt.Errorf("create application clients: %w", err)
	}
	if err := applyStartupCRD(
		ctx, cfg, clients.Dynamic,
	); err != nil {
		return nil, fmt.Errorf("apply startup CRD configuration: %w", err)
	}
	// The CRD overlay may change monitor, alert, or startup settings. Rebuild
	// the immutable snapshot before composing any domain component.
	runtime = config.RuntimeConfigFor(cfg)
	upgraderConfig = runtime.Upgrader()

	persistenceManager := persistence.NewManagerWithClock(
		clients.Kubernetes, k8s.GetNamespace(), clock.Func(now),
	)
	startupManager := startup.NewStartupManagerWithRuntime(
		persistenceManager,
		runtime,
		clock.Func(now),
	)
	startupResult, err := startupManager.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("run startup: %w", err)
	}

	healthServer := health.NewHealthServer(runtime.HealthCheck())
	securityMonitor := configureSecurityMonitor(runtime, clients.Kubernetes, now)
	healthServer.SetSecurityLister(securityMonitor)

	deliveryManager := delivery.NewManagerWithDependencies(delivery.Dependencies{
		HTTPClient: clients.HTTP,
		Clock:      clock.Func(now),
	})
	deliveryManager.InitRuntime(runtime, catalog.NewProvider)
	configureDelivery(ctx, deliveryManager)

	upgrader := upgrader.NewUpgrader(
		&upgraderConfig,
		deliveryManager,
		persistenceManager,
		clients.HTTP,
	)
	telemetryRun := configureTelemetryRunner(
		runtime.Telemetry(),
		persistenceManager,
		startupResult.ClusterID,
		startupResult.CurrentVersion,
		now,
		clients.HTTP,
	)

	return &bootstrap{
		runtime:         runtime,
		clients:         clients,
		persistence:     persistenceManager,
		startupManager:  startupManager,
		startupResult:   startupResult,
		healthServer:    healthServer,
		securityMonitor: securityMonitor,
		deliveryManager: deliveryManager,
		telemetryRun:    telemetryRun,
		upgradeRun:      upgrader.CheckUpdates,
	}, nil
}
