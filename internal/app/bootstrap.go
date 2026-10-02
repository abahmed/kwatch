package app

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/abahmed/kwatch/internal/alert/catalog"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	inventorykube "github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/rbac"
)

// bootstrap is the infrastructure every replica builds before the Lease is
// acquired: clients, health, delivery. Nothing here writes state; that
// belongs to the active session.
type bootstrap struct {
	runtime         config.RuntimeConfig
	clients         kubeclient.ClientSet
	healthServer    *health.HealthServer
	deliveryManager *delivery.Manager
	securityMonitor *rbac.Monitor
	heartbeat       *heartbeat.HeartbeatMonitor
	clock           clock.Clock
	threadWake      *threadWake
}

func newBootstrap(
	ctx context.Context,
	cfg *config.Config,
	now func() time.Time,
) (*bootstrap, error) {
	clockSource := clock.Func(now)
	clients, err := kubeclient.NewClusterClientSet(&net.Resolver{}, clockSource)
	if err != nil {
		return nil, fmt.Errorf("create application clients: %w", err)
	}
	cfg, overlayInvalid, err := applyStartupCRD(
		ctx, cfg, clients.Dynamic, loadConfig)
	if err != nil {
		return nil, fmt.Errorf("apply startup CRD configuration: %w", err)
	}
	// Report warnings once, after the overlay, so KwatchConfig typos are
	// reported next to the file's.
	logConfigWarnings(cfg)
	// The overlay may change monitor and alert settings, so the snapshot
	// and the clients that depend on it (outbound HTTP, kubelet) are built
	// once, here.
	runtime := config.RuntimeConfigFor(cfg)
	clients, err = clients.WithRuntime(runtime)
	if err != nil {
		return nil, fmt.Errorf("create configured clients: %w", err)
	}

	wake := &threadWake{}
	deliveryManager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{
			HTTPClient: clients.HTTP, Clock: clockSource,
			OnDelivered: wake.Notify,
		},
	)
	if err := deliveryManager.InitRuntime(
		runtime, catalog.NewProvider,
	); err != nil {
		return nil, fmt.Errorf("initialize delivery providers: %w", err)
	}
	healthServer := health.NewHealthServerWithClock(
		runtime.Lifecycle().HealthCheck(), clockSource,
	)
	reportOverlayHealth(healthServer, overlayInvalid)
	return &bootstrap{
		runtime:         runtime,
		clients:         clients,
		healthServer:    healthServer,
		deliveryManager: deliveryManager,
		securityMonitor: rbac.NewMonitor(clients.Kubernetes,
			permissionChecks(runtime), clockSource,
			reportPermissions(healthServer)),
		// The heartbeat pings only while monitoring is ready.
		heartbeat: heartbeat.NewHeartbeatMonitorWithRuntime(
			runtime, clients.HTTP, healthServer.Ready),
		clock:      clockSource,
		threadWake: wake,
	}, nil
}

// permissionChecks is every permission the audit verifies. With
// watch.secrets false kwatch is deliberately granted no Secret access, so
// the audit does not expect it.
func permissionChecks(runtime config.RuntimeConfig) []inventorykube.Access {
	checks := rbac.Checks(kubeclient.GetNamespace(), electionLeaseName(),
		runtime.Lifecycle().CRDEnabled())
	if !runtime.Application().WatchSecrets {
		checks = inventorykube.WithoutResource(checks,
			inventorykube.SecretsResource)
	}
	return checks
}
