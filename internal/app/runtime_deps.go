package app

import (
	"context"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/pvc"
)

func makeServerDeps(
	ctx context.Context,
	cancel context.CancelFunc,
	runtime config.RuntimeConfig,
	boot *bootstrap,
	ctl *controller.Controller,
	cleanup func(),
	persist persistenceSetup,
	incidentEngine *incident.Engine,
	pvcMonitor *pvc.PvcMonitor,
	hbMonitor *heartbeat.HeartbeatMonitor,
	optional optionalRuns,
	auditLogger *audit.AuditLogger,
	startupSummary func(map[string]int),
	initialized <-chan struct{},
	readiness *readinessCoordinator,
) *serverDeps {
	deps := &serverDeps{
		ctx: ctx, cancel: cancel, runtime: runtime, clients: boot.clients,
		healthServer:    boot.healthServer,
		readiness:       readiness,
		deliveryManager: boot.deliveryManager,
		incidentEngine:  incidentEngine, pvcMonitor: pvcMonitor,
		hbMonitor: hbMonitor, ctl: ctl,
		incidentCh: persist.incidentCh, incidentSaver: persist.incidentSaver,
		incidentDone: persist.incidentDone, baselineDone: persist.baselineDone,
		changeDone: persist.changeDone, feedbackDone: persist.feedbackDone,
		startPersistence: persist.start,
		initialized:      initialized, controllerDone: make(chan struct{}),
		controllerProgress: newComponentProgress(boot.clock.Now()),
		notifyStartup: func() {
			if msg, ok := boot.startupManager.StartupMessage(); ok {
				boot.deliveryManager.Notify(msg)
			}
		},
		recordAlive: boot.startupManager.RecordAlive,
		closeAudit:  auditLogger.Close, cleanup: cleanup,
		tlsSweep: optional.tlsSweep, statusRun: optional.statusRun,
		metricsRun: optional.metricsRun, probeRun: optional.probeRun,
		kubeletRun: optional.kubeletRun, storageRun: optional.storageRun,
		networkRun: optional.networkRun, securityRun: optional.securityRun,
		controlPlaneRun:      optional.controlPlaneRun,
		telemetryRun:         boot.telemetryRun,
		upgradeRun:           boot.upgradeRun,
		notifyStartupSummary: startupSummary,
	}
	deps.persistenceGate = newPersistenceGate()
	return deps
}
