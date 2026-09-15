package app

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/delivery"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/persistence"
	"github.com/abahmed/kwatch/internal/pvc"
)

// buildServerDeps composes domain engines, monitor families, and lifecycle
// dependencies after infrastructure bootstrap has completed.
func buildServerDeps(
	ctx context.Context,
	cancel context.CancelFunc,
	runtime config.RuntimeConfig,
	boot *bootstrap,
	now func() time.Time,
) (*serverDeps, error) {
	persist := configurePersistence(ctx, boot.persistence, now)
	boot.healthServer.SetPersistenceLister(boot.persistence)
	if report := boot.persistence.MigrationReport(); len(report) > 0 {
		migrationState := "running"
		migrationReason := ""
		migrationAvailable := true
		for _, result := range report {
			if result.Status == persistence.MigrationFailed ||
				result.Status == persistence.MigrationUnsupported {
				migrationState = "degraded"
				migrationReason = "persistence_migration_failed"
				migrationAvailable = false
				break
			}
		}
		boot.healthServer.SetComponentStatus(
			"persistence", migrationState, migrationReason,
			migrationAvailable,
		)
	}
	graph := kwcontext.NewResourceGraph()
	auditLogger := audit.NewLogger(audit.Config{
		Enabled: runtime.AuditLog().Enabled,
		Output:  runtime.AuditLog().Output,
		Now:     now,
	})

	var incidentEngine *incident.Engine
	activeChecker := newActiveGraphKeyChecker(
		func() map[model.IncidentKey]*model.Incident {
			if incidentEngine == nil {
				return nil
			}
			return incidentEngine.ActiveIncidents()
		}, now,
	)
	insightEngine := insight.NewEngineWithDependencies(
		graph, persist.tracker, insight.Dependencies{
			Clock:         clock.Func(now),
			FeedbackStore: persist.feedbackStore,
			ActiveChecker: activeChecker,
		},
	)
	var ctl *controller.Controller
	incidentEngine = newIncidentEngine(
		runtime,
		now,
		func(resource, namespace, name string) (bool, bool) {
			if ctl == nil {
				return false, false
			}
			return ctl.ResourceExists(resource, namespace, name)
		},
		persist.baseline,
		boot.deliveryManager,
		auditLogger,
		graph,
		persist.baselineCh,
		insightEngine,
		persist.feedbackStore,
		persist.saveFeedback,
	)
	configureIncidentHealth(
		boot.healthServer, incidentEngine, boot.deliveryManager,
	)

	pvcMonitor := pvc.NewPvcMonitorWithRuntimeAndClock(
		boot.clients.Kubernetes,
		runtime,
		incidentEngine,
		boot.persistence,
		clock.Func(now),
	)
	hbMonitor := heartbeat.NewHeartbeatMonitorWithRuntime(
		runtime, boot.clients.HTTP,
	)
	monitors := composeMonitorComponents(
		runtime,
		boot.clients.Kubernetes,
		boot.clients,
		incidentEngine,
		boot.healthServer,
		boot.deliveryManager,
		now,
	)

	var cleanup func()
	var err error
	ctl, cleanup, err = newMonitorController(
		boot.clients.Kubernetes, runtime, monitors, now,
	)
	if err != nil {
		closeAuditLogger(auditLogger)
		return nil, fmt.Errorf("create controller: %w", err)
	}
	initialized := configureControllerRuntime(
		ctx,
		runtime,
		boot,
		ctl,
		persist,
		incidentEngine,
		pvcMonitor,
		graph,
	)
	if err := boot.healthServer.Open(); err != nil {
		cleanup()
		closeAuditLogger(auditLogger)
		return nil, fmt.Errorf("start health check server: %w", err)
	}
	optional := configureOptionalRuns(
		runtime,
		boot,
		ctl,
		graph,
		incidentEngine,
		monitors,
		now,
	)
	return makeServerDeps(
		ctx,
		cancel,
		runtime,
		boot,
		ctl,
		cleanup,
		persist,
		incidentEngine,
		pvcMonitor,
		hbMonitor,
		optional,
		auditLogger,
		monitors.startupSummary,
		initialized,
	), nil
}

func configureIncidentHealth(
	healthServer *health.HealthServer,
	incidentEngine *incident.Engine,
	deliveryManager *delivery.Manager,
) {
	healthServer.SetIncidentAPI(incidentEngine)
	healthServer.SetDeliveryManager(deliveryManager)
	healthServer.SetDeadLetterLister(deliveryManager)
}

func closeAuditLogger(logger *audit.AuditLogger) {
	if err := logger.Close(); err != nil {
		klog.ErrorS(err, "failed to close audit logger")
	}
}
