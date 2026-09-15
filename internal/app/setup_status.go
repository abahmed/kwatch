package app

import (
	"context"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/statuswatch"
)

func configureStatusMonitor(
	runtime config.RuntimeConfig,
	ctl *controller.Controller,
	graph *kwcontext.ResourceGraph,
	incidentSink monitor.ObservationSink,
	healthServer *health.HealthServer,
	now func() time.Time,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.DiscoveryInterface,
) func(context.Context) {
	if !runtime.ClusterResourceMonitor().Enabled {
		return nil
	}
	statusMonitor := statuswatch.NewWithClientsAndClock(
		dynamicClient, discoveryClient, incidentSink,
		runtime.ResyncInterval(), clock.Func(now),
	)
	namespaces, watchAll := ctl.NamespaceScope()
	if err := statusMonitor.ConfigureSources(statuswatch.Sources{
		NamespaceAllowed: ctl.NamespaceAllowed,
		Namespaces:       namespaces, WatchAll: watchAll,
		Graph: graph, ServiceLister: ctl.ServiceLister(),
	}); err != nil {
		healthServer.SetComponentError("status", err)
		return nil
	}
	if err := statusMonitor.SetConditionRules(
		runtime.CrdConfig().FailureConditions,
	); err != nil {
		healthServer.SetComponentError("status", err)
		klog.ErrorS(err, "invalid generic status condition rules")
		return nil
	}
	if err := statusMonitor.SetGraphReferenceRules(
		runtime.CrdConfig().GraphReferences,
	); err != nil {
		healthServer.SetComponentError("status", err)
		klog.ErrorS(err, "invalid generic status graph reference rules")
		return nil
	}
	return func(ctx context.Context) {
		if err := statusMonitor.Start(ctx); err != nil {
			healthServer.SetComponentError("status", err)
			klog.ErrorS(err, "generic status monitor stopped")
			return
		}
		status := statusMonitor.Status()
		if len(status.SkippedResources) > 0 {
			healthServer.SetComponentStatus(
				"status", "degraded", "optional_api_unavailable", false,
			)
			return
		}
		healthServer.SetComponentStatus("status", "running", "", true)
	}
}
