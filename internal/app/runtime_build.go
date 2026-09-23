package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
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
	readiness := newReadinessCoordinator(boot.healthServer)
	persist := configurePersistence(
		boot.persistence, runtime, now, boot.healthServer, readiness,
	)
	graph := kwcontext.NewResourceGraph()
	auditLogger := audit.NewLogger(audit.Config{
		Enabled: runtime.Lifecycle().AuditLog().Enabled,
		Output:  runtime.Lifecycle().AuditLog().Output,
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
	initialized := make(chan struct{})
	var readyOnce sync.Once
	ready := func() {
		status := ctl.InformerStatus()
		if len(status.UnavailableSources) > 0 {
			boot.healthServer.SetComponentStatus(
				"informer", "degraded", "source_not_configured", false,
			)
			readiness.setCurrent("controller", false)
			return
		}
		boot.healthServer.SetComponentStatus(
			"informer", "running", "", true,
		)
		readiness.setCurrent("controller", true)
		readyOnce.Do(func() { close(initialized) })
	}
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
		boot.deliveryManager,
		now,
		func() bool { return boot.startupResult.ShouldNotify },
	)

	var cleanup func()
	var err error
	ctl, cleanup, err = newMonitorController(
		boot.clients.Kubernetes, runtime, monitors,
		controller.RuntimeDependencies{
			Context: ctx,
			Tracker: persist.tracker,
			Graph:   graph,
			Ready:   ready,
			Now:     now,
		},
	)
	if err != nil {
		closeAuditLogger(auditLogger)
		return nil, fmt.Errorf("create controller: %w", err)
	}
	if err := configureControllerRuntime(
		runtime,
		boot,
		ctl,
		pvcMonitor,
	); err != nil {
		cleanup()
		closeAuditLogger(auditLogger)
		return nil, err
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
	if err := boot.healthServer.ConfigureDependencies(health.Dependencies{
		Incident:          incidentEngine,
		Delivery:          boot.deliveryManager,
		DeadLetters:       boot.deliveryManager,
		Telemetry:         optional.telemetry,
		AdoptionTelemetry: boot.telemetryStatus,
		Security:          boot.securityMonitor,
		ControlPlane:      monitors.controlPlane,
		Informer:          ctl,
		Persistence:       boot.persistence,
	}); err != nil {
		cleanup()
		closeAuditLogger(auditLogger)
		return nil, fmt.Errorf("configure health dependencies: %w", err)
	}
	if err := boot.healthServer.Open(); err != nil {
		cleanup()
		closeAuditLogger(auditLogger)
		return nil, fmt.Errorf("start health check server: %w", err)
	}
	deps := makeServerDeps(
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
		readiness,
	)
	deps.activate = func(activeCtx context.Context) error {
		if err := persist.activate(
			activeCtx, incidentEngine.SetBaseline,
		); err != nil {
			readiness.setCurrent("restore", false)
			return fmt.Errorf("activate persistence: %w", err)
		}
		if err := restoreControllerRuntime(
			activeCtx, boot, ctl, incidentEngine,
		); err != nil {
			readiness.setCurrent("restore", false)
			return err
		}
		if err := boot.activate(activeCtx); err != nil {
			readiness.setCurrent("restore", false)
			return err
		}
		readiness.setCurrent("restore", true)
		readiness.setCurrent("incident", true)
		deps.telemetryRun = boot.telemetryRun
		return nil
	}
	return deps, nil
}

func closeAuditLogger(logger *audit.AuditLogger) {
	if err := logger.Close(); err != nil {
		klog.ErrorS(err, "failed to close audit logger")
	}
}
