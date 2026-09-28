package app

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/k8s/dynamicwatch"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/networkgraph"
	"github.com/abahmed/kwatch/internal/storagegraph"
)

const optionalWatcherSyncTimeout = 30 * time.Second

func newNetworkGraphRun(
	runtime config.RuntimeConfig,
	graph *kwcontext.ResourceGraph,
	ctl *controller.Controller,
	healthServer *health.HealthServer,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.DiscoveryInterfaceWithContext,
) func(context.Context) error {
	if !runtime.Monitors().ClusterResource().Enabled ||
		(!runtime.Monitors().Service().Enabled &&
			!runtime.Monitors().Ingress().Enabled &&
			!runtime.Monitors().NetworkPolicy().Enabled) {
		return nil
	}
	graphMonitor := networkgraph.NewWithClients(
		dynamicClient, discoveryClient, graph,
		runtime.Lifecycle().ResyncInterval(),
	)
	namespaces, watchAll := ctl.NamespaceScope()
	if err := graphMonitor.ConfigureSources(networkgraph.Sources{
		Allowed: ctl.NamespaceAllowed, Namespaces: namespaces,
		WatchAll: watchAll, Graph: graph,
	}); err != nil {
		healthServer.SetComponentError("network-graph", err)
		return nil
	}
	return func(ctx context.Context) error {
		return runOptionalGraph(ctx, graphMonitor, healthServer, "network-graph")
	}
}

func newStorageGraphRun(
	runtime config.RuntimeConfig,
	graph *kwcontext.ResourceGraph,
	incidentSink monitor.ObservationSink,
	ctl *controller.Controller,
	healthServer *health.HealthServer,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.DiscoveryInterfaceWithContext,
) func(context.Context) error {
	if !runtime.Monitors().ClusterResource().Enabled {
		return nil
	}
	graphMonitor := storagegraph.NewWithClients(
		dynamicClient, discoveryClient, graph,
		runtime.Lifecycle().ResyncInterval(),
	)
	namespaces, watchAll := ctl.NamespaceScope()
	if err := graphMonitor.ConfigureSources(storagegraph.Sources{
		Allowed: ctl.NamespaceAllowed, Namespaces: namespaces,
		WatchAll: watchAll, Graph: graph,
		ObservationSink: incidentSink,
	}); err != nil {
		healthServer.SetComponentError("storage-graph", err)
		return nil
	}
	return func(ctx context.Context) error {
		return runOptionalGraph(ctx, graphMonitor, healthServer, "storage-graph")
	}
}

// optionalRediscoveryInterval is how often a degraded optional watcher checks
// whether its missing APIs were installed after startup.
var optionalRediscoveryInterval = 5 * time.Minute

type optionalGraphMonitor interface {
	Start(context.Context) error
	Stop(context.Context) error
	WaitForCacheSync(context.Context) bool
	Status() dynamicwatch.Status
	Rediscover(context.Context) (bool, error)
}

// runOptionalGraph owns one optional dynamic watcher: start, cache sync,
// health, periodic rediscovery of late-installed APIs, and bounded stop.
func runOptionalGraph(
	ctx context.Context,
	graphMonitor optionalGraphMonitor,
	healthServer *health.HealthServer,
	name string,
) error {
	if err := graphMonitor.Start(ctx); err != nil {
		healthServer.SetComponentError(name, err)
		klog.ErrorS(err, "optional graph monitor stopped", "component", name)
		return err
	}
	defer func() {
		stopCtx, cancel := boundedShutdownContext(ctx)
		defer cancel()
		if err := graphMonitor.Stop(stopCtx); err != nil {
			healthServer.SetComponentError(name, err)
		}
	}()
	if err := syncOptionalGraph(
		ctx, graphMonitor, healthServer, name,
	); err != nil {
		return err
	}
	ticker := time.NewTicker(optionalRediscoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		if graphMonitor.Status().Skipped == 0 {
			continue
		}
		replaced, err := graphMonitor.Rediscover(ctx)
		if err != nil {
			healthServer.SetComponentError(name, err)
			return err
		}
		if !replaced {
			continue
		}
		klog.InfoS("optional API installed; watcher restarted",
			"component", name)
		if err := syncOptionalGraph(
			ctx, graphMonitor, healthServer, name,
		); err != nil {
			return err
		}
	}
}

func syncOptionalGraph(
	ctx context.Context,
	graphMonitor optionalGraphMonitor,
	healthServer *health.HealthServer,
	name string,
) error {
	syncCtx, cancel := context.WithTimeout(ctx, optionalWatcherSyncTimeout)
	defer cancel()
	if !graphMonitor.WaitForCacheSync(syncCtx) {
		err := fmt.Errorf("optional watcher cache sync failed")
		healthServer.SetComponentError(name, err)
		klog.ErrorS(err, "optional graph watcher degraded", "component", name)
		return err
	}
	if graphMonitor.Status().Skipped > 0 {
		healthServer.SetComponentStatus(
			name, "degraded", "optional_api_unavailable", false,
		)
		return nil
	}
	healthServer.SetComponentStatus(name, "running", "", true)
	return nil
}
