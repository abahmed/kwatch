package app

import (
	"context"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/rbac"
)

func configureControllerRuntime(
	ctx context.Context,
	runtime config.RuntimeConfig,
	boot *bootstrap,
	ctl *controller.Controller,
	persist persistenceSetup,
	incidentEngine *incident.Engine,
	pvcMonitor *pvc.PvcMonitor,
	graph *kwcontext.ResourceGraph,
) <-chan struct{} {
	boot.healthServer.SetInformerLister(ctl)
	namespaces, watchAll := ctl.NamespaceScope()
	if err := boot.securityMonitor.ConfigureSources(rbac.Sources{
		Namespaces: namespaces, AllNamespaces: watchAll,
		InfrastructureNamespace: k8s.GetNamespace(),
	}); err != nil {
		boot.healthServer.SetComponentError("rbac", err)
	}
	if err := pvcMonitor.ConfigureSources(pvc.Sources{
		Allowed: namespaces, Forbidden: runtime.ForbiddenNamespaces(),
		WatchAll: watchAll, NamespaceAllowed: ctl.NamespaceAllowed,
	}); err != nil {
		boot.healthServer.SetComponentError("pvc", err)
	}
	if persist.incidentSaver != nil {
		restoreIncidents(
			ctx,
			boot.persistence,
			incidentEngine,
			ctl.NamespaceAllowed,
		)
		restoreProviderThreads(
			ctx,
			boot.persistence,
			boot.deliveryManager,
			incidentEngine,
		)
		restoreEngineState(ctx, boot.persistence, incidentEngine)
	}
	ctl.SetTracker(persist.tracker)
	ctl.SetGraph(graph)
	initialized := make(chan struct{})
	ctl.SetReadyFunc(func() {
		status := ctl.InformerStatus()
		if len(status.UnavailableSources) > 0 {
			boot.healthServer.SetComponentStatus(
				"informer", "degraded", "source_not_configured", false,
			)
			boot.healthServer.SetReady(false)
			return
		}
		boot.healthServer.SetComponentStatus(
			"informer", "running", "", true,
		)
		boot.healthServer.SetReady(true)
		close(initialized)
	})
	return initialized
}
