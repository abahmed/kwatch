package app

import (
	"context"
	"fmt"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/persistence"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/rbac"
)

func configureControllerRuntime(
	runtime config.RuntimeConfig,
	boot *bootstrap,
	ctl *controller.Controller,
	pvcMonitor *pvc.PvcMonitor,
) error {
	namespaces, watchAll := ctl.NamespaceScope()
	if err := boot.securityMonitor.ConfigureSources(rbac.Sources{
		Namespaces: namespaces, AllNamespaces: watchAll,
		InfrastructureNamespace: k8s.GetNamespace(),
	}); err != nil {
		boot.healthServer.SetComponentError("rbac", err)
		return fmt.Errorf("configure RBAC sources: %w", err)
	}
	if err := pvcMonitor.ConfigureSources(pvc.Sources{
		Allowed:   namespaces,
		Forbidden: runtime.Scope().ForbiddenNamespaces(),
		WatchAll:  watchAll, NamespaceAllowed: ctl.NamespaceAllowed,
	}); err != nil {
		boot.healthServer.SetComponentError("pvc", err)
		return fmt.Errorf("configure PVC sources: %w", err)
	}
	return nil
}

func restoreControllerRuntime(
	ctx context.Context,
	boot *bootstrap,
	ctl *controller.Controller,
	incidentEngine *incident.Engine,
) error {
	if err := restoreIncidents(
		ctx, boot.persistence, incidentEngine, ctl.NamespaceAllowed,
	); err != nil {
		recordRestoreResult(boot.persistence, "incidents", err)
		return fmt.Errorf("restore incidents: %w", err)
	}
	recordRestoreResult(boot.persistence, "incidents", nil)
	if err := restoreGroups(ctx, boot.persistence, incidentEngine); err != nil {
		recordRestoreResult(boot.persistence, "groups", err)
		return fmt.Errorf("restore groups: %w", err)
	}
	recordRestoreResult(boot.persistence, "groups", nil)
	if err := restoreProviderThreads(
		ctx, boot.persistence, boot.deliveryManager, incidentEngine,
	); err != nil {
		recordRestoreResult(boot.persistence, "threads", err)
		return fmt.Errorf("restore provider threads: %w", err)
	}
	recordRestoreResult(boot.persistence, "threads", nil)
	if err := restoreEngineState(
		ctx, boot.persistence, incidentEngine,
	); err != nil {
		recordRestoreResult(boot.persistence, "engine", err)
		return fmt.Errorf("restore engine state: %w", err)
	}
	recordRestoreResult(boot.persistence, "engine", nil)
	return nil
}

func recordRestoreResult(
	manager *persistence.Manager,
	store string,
	err error,
) {
	status := persistence.MigrationCompleted
	detail := "restored persisted state"
	if err != nil {
		status = persistence.MigrationFailed
		detail = "persisted state restore failed"
	}
	manager.RecordMigrationResult(persistence.MigrationResult{
		Store:                 store,
		Operation:             persistence.OperationRestore,
		SourceFormat:          "kwatch-" + store,
		DestinationFormat:     "runtime/" + store,
		Status:                status,
		Recoverable:           err == nil,
		MonitoringMayContinue: err == nil,
		Detail:                detail,
	}, err)
}
