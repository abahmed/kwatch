package app

import (
	"context"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
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
) {
	namespaces, watchAll := ctl.NamespaceScope()
	if err := boot.securityMonitor.ConfigureSources(rbac.Sources{
		Namespaces: namespaces, AllNamespaces: watchAll,
		InfrastructureNamespace: k8s.GetNamespace(),
	}); err != nil {
		boot.healthServer.SetComponentError("rbac", err)
	}
	if err := pvcMonitor.ConfigureSources(pvc.Sources{
		Allowed:   namespaces,
		Forbidden: runtime.Scope().ForbiddenNamespaces(),
		WatchAll:  watchAll, NamespaceAllowed: ctl.NamespaceAllowed,
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
}
