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
		if err := graphMonitor.Start(ctx); err != nil {
			healthServer.SetComponentError("network-graph", err)
			klog.ErrorS(err, "network graph monitor stopped")
			return err
		}
		defer func() {
			stopCtx, cancel := context.WithTimeout(
				context.Background(), componentShutdownTimeout,
			)
			defer cancel()
			if err := graphMonitor.Stop(stopCtx); err != nil {
				healthServer.SetComponentError("network-graph", err)
			}
		}()
		syncCtx, cancel := context.WithTimeout(
			ctx, optionalWatcherSyncTimeout,
		)
		defer cancel()
		if !graphMonitor.WaitForCacheSync(syncCtx) {
			err := fmt.Errorf("optional network watcher cache sync failed")
			healthServer.SetComponentError("network-graph", err)
			klog.ErrorS(err, "network graph watcher degraded")
			return err
		}
		if status := graphMonitor.Status(); status.Skipped > 0 {
			healthServer.SetComponentStatus(
				"network-graph",
				"degraded", "optional_api_unavailable", false,
			)
			<-ctx.Done()
			return nil
		}
		healthServer.SetComponentStatus(
			"network-graph", "running", "", true,
		)
		<-ctx.Done()
		return nil
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
		if err := graphMonitor.Start(ctx); err != nil {
			healthServer.SetComponentError("storage-graph", err)
			klog.ErrorS(err, "storage graph monitor stopped")
			return err
		}
		defer func() {
			stopCtx, cancel := context.WithTimeout(
				context.Background(), componentShutdownTimeout,
			)
			defer cancel()
			if err := graphMonitor.Stop(stopCtx); err != nil {
				healthServer.SetComponentError("storage-graph", err)
			}
		}()
		syncCtx, cancel := context.WithTimeout(
			ctx, optionalWatcherSyncTimeout,
		)
		defer cancel()
		if !graphMonitor.WaitForCacheSync(syncCtx) {
			err := fmt.Errorf("optional storage watcher cache sync failed")
			healthServer.SetComponentError("storage-graph", err)
			klog.ErrorS(err, "storage graph watcher degraded")
			return err
		}
		if status := graphMonitor.Status(); status.Skipped > 0 {
			healthServer.SetComponentStatus(
				"storage-graph",
				"degraded", "optional_api_unavailable", false,
			)
			<-ctx.Done()
			return nil
		}
		healthServer.SetComponentStatus(
			"storage-graph", "running", "", true,
		)
		<-ctx.Done()
		return nil
	}
}
