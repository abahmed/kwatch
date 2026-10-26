package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert/catalog"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/dashboard"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/feature"
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
	dashboardServer *dashboard.Server
	deliveryManager *delivery.Manager
	securityMonitor *rbac.Monitor
	heartbeat       *heartbeat.HeartbeatMonitor
	clock           clock.Clock
	threadWake      *threadWake
	// entitlements is what this installation may run, created once at
	// startup. inactivePro names the Pro features the configuration sets
	// that it does not run.
	entitlements feature.Entitlements
	inactivePro  []string
	// notice is what the edition does not run (detectors, provider cap).
	notice editionNotice
	// license is the licensing wiring built from the configuration.
	license *licenseSetup
	// overlay is how the startup KwatchConfig turned out.
	overlay overlayOutcome
}

func newBootstrap(
	ctx context.Context,
	cfg *config.Config,
	now func() time.Time, processStart time.Time,
	entitlements feature.Entitlements,
) (*bootstrap, error) {
	clockSource := clock.Func(now)
	clients, err := kubeclient.NewClusterClientSet(&net.Resolver{}, clockSource)
	if err != nil {
		return nil, fmt.Errorf("create application clients: %w", err)
	}
	cfg, overlay, err := applyStartupCRD(
		ctx, cfg, clients.Dynamic, loadConfig, func() ([]byte, bool) {
			return readLastGood(StatePath(), clockSource.Now)
		})
	if err != nil {
		return nil, fmt.Errorf("apply startup CRD configuration: %w", err)
	}
	// Report warnings once, after the overlay, so KwatchConfig typos are
	// reported next to the file's.
	logConfigWarnings(cfg)
	// Dropped after the overlay, so a KwatchConfig cannot enable Pro.
	notice := newEditionNotice(entitlements, cfg)
	cfg, inactive := applyEdition(cfg, entitlements)
	// The overlay may change monitor and alert settings, so the snapshot
	// and the clients that depend on it (outbound HTTP, kubelet) are built
	// once, here.
	runtime := config.RuntimeConfigFor(cfg)
	clients, err = clients.WithRuntime(runtime)
	if err != nil {
		return nil, fmt.Errorf("create configured clients: %w", err)
	}

	wake := &threadWake{}
	deliveryManager, err := newDeliveryManager(
		runtime, clients, clockSource, wake, entitlements)
	if err != nil {
		return nil, err
	}
	healthServer := newHealthServer(
		runtime, clockSource, overlay.invalid, entitlements)
	dashboardServer := newDashboardServer(runtime, entitlements)
	return &bootstrap{
		runtime:         runtime,
		clients:         clients,
		healthServer:    healthServer,
		dashboardServer: dashboardServer,
		deliveryManager: deliveryManager,
		securityMonitor: rbac.NewMonitor(clients.Kubernetes,
			permissionChecks(runtime), clockSource,
			reportPermissions(healthServer)),
		// The heartbeat pings only while monitoring is ready.
		heartbeat: heartbeat.NewHeartbeatMonitorWithRuntime(
			runtime, clients.HTTP, healthServer.Ready),
		clock: clockSource, threadWake: wake,
		entitlements: entitlements, inactivePro: inactive, notice: notice,
		license: newLicenseSetup(runtime, processStart),
		overlay: overlay,
	}, nil
}

// newDeliveryManager builds the delivery manager and its providers.
func newDeliveryManager(
	runtime config.RuntimeConfig, clients kubeclient.ClientSet,
	clockSource clock.Clock, wake *threadWake,
	entitlements feature.Entitlements,
) (*delivery.Manager, error) {
	manager := delivery.NewManagerWithDependencies(
		delivery.Dependencies{
			HTTPClient: clients.HTTP, Clock: clockSource,
			OnDelivered: wake.Notify, Entitlements: entitlements,
		},
	)
	if err := manager.InitRuntime(
		runtime, catalog.NewProvider,
	); err != nil {
		return nil, fmt.Errorf("initialize delivery providers: %w", err)
	}
	return manager, nil
}

// permissionChecks is every permission the audit verifies. With
// watch.secrets false kwatch is deliberately granted no Secret access, so
// the audit does not expect it, except the license instance Secret.
func permissionChecks(runtime config.RuntimeConfig) []inventorykube.Access {
	checks := rbac.Checks(kubeclient.GetNamespace(), electionLeaseName(),
		runtime.Lifecycle().CRDEnabled())
	if !runtime.Application().WatchSecrets {
		checks = inventorykube.WithoutResource(checks,
			inventorykube.SecretsResource)
	}
	// The license instance Secret is a grant of its own, kept in every
	// mode, so it is checked whenever a key is configured, even with
	// watch.secrets false.
	if runtime.Lifecycle().License().Key != "" {
		checks = append(checks, rbac.LicenseChecks(
			kubeclient.GetNamespace(), licenseInstanceSecret)...)
	}
	return checks
}

// newHealthServer builds the health server for the edition: the JSON
// /status format and /metrics are Pro features.
func newHealthServer(
	runtime config.RuntimeConfig, now clock.Clock, overlayInvalid bool,
	entitlements feature.Entitlements,
) *health.HealthServer {
	server := health.NewHealthServerWithClock(
		runtime.Lifecycle().HealthCheck(), now)
	reportOverlayHealth(server, overlayInvalid)
	applyEditionEndpoints(server, entitlements)
	if !feature.OrCommunity(entitlements).Limits().Metrics {
		klog.InfoS(health.MetricsProText, "component", "health",
			"operation", "start")
	}
	return server
}

// applyEditionEndpoints opens or closes the Pro endpoints of the health
// server for e: the JSON /status format and /metrics. A nil e is
// Community.
func applyEditionEndpoints(
	server *health.HealthServer, e feature.Entitlements,
) {
	limits := feature.OrCommunity(e).Limits()
	if limits.StatusJSON {
		server.AllowStatusJSON()
	} else {
		server.DenyStatusJSON()
	}
	if limits.Metrics {
		server.AllowMetrics()
	} else {
		server.DenyMetrics()
	}
}

// newDashboardServer builds the optional dashboard API server. It is Pro
// only and off by default; the listener is opened later by the generation.
// The bearer token comes from KWATCH_DASHBOARD_TOKEN (populated by the
// installer/chart from the kwatch-dashboard Secret).
func newDashboardServer(
	runtime config.RuntimeConfig, e feature.Entitlements,
) *dashboard.Server {
	limits := feature.OrCommunity(e).Limits()
	if !limits.Dashboard {
		return nil
	}
	dc := runtime.Lifecycle().Dashboard()
	if !dc.Enabled {
		return nil
	}
	token := os.Getenv("KWATCH_DASHBOARD_TOKEN")
	edition := string(feature.CommunityEdition)
	if e != nil {
		edition = string(e.Edition())
	}
	return dashboard.NewServer(
		true, dc.Port, token, edition,
		nil, nil, // incidents and status set later by the pipeline
	)
}
