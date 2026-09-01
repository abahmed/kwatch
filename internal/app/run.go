package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/feature"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/kubeletmetrics"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/upgrader"
	"github.com/abahmed/kwatch/internal/version"
)

// serverDeps bundles the wired components so the background loop and the
// shutdown sequence can live in their own functions.
type serverDeps struct {
	ctx           context.Context
	cancel        context.CancelFunc
	cfg           *config.Config
	healthServer  *health.HealthServer
	alertManager  *alert.AlertManager
	correlator    *correlation.Engine
	pvcMonitor    *pvc.PvcMonitor
	hbMonitor     *heartbeat.HeartbeatMonitor
	ctl           *controller.Controller
	incidentCh    chan []model.PersistedIncident
	incidentSaver incidentSaver
	incidentDone  <-chan struct{}
	feedbackDone  <-chan struct{}
	notifyStartup func()
	// recordAlive stamps the liveness marker that lets the next start report
	// how long monitoring was down.
	recordAlive     func(context.Context)
	closeAudit      func() error
	cleanup         func()
	tlsSweep        func()
	statusRun       func(context.Context)
	metricsRun      func(context.Context)
	probeRun        func(context.Context)
	kubeletRun      func(context.Context)
	storageRun      func(context.Context)
	networkRun      func(context.Context)
	securityRun     func(context.Context)
	controlPlaneRun func(context.Context)
	telemetryRun    func(context.Context)
}

// Run loads config and wires all monitors, then runs until shutdown.
func Run() int {
	return RunWithClock(time.Now)
}

// RunWithClock runs kwatch with an injected clock, primarily for deterministic
// integration tests and embedded callers.
func RunWithClock(now func() time.Time) int {
	clock.Set(now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, featurePlan, err := loadConfigAndFeaturePlan(now())
	if err != nil {
		klog.ErrorS(err, "failed to load config and build feature plan")
		return 1
	}
	cfg.WatchStartTime = now()
	// Install the plan before startup handling because startup failures may
	// already produce a notification through the alert manager.
	message.SetFeaturePlan(featurePlan)

	klog.InfoS(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	if err := applyStartupCRD(ctx, cfg); err != nil {
		klog.ErrorS(err, "failed to apply startup CRD configuration")
		return 1
	}
	k8s.InitHTTPClient(&cfg.App)
	k8sClient, err := client.NewKubernetesClient(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create kubernetes client")
		return 1
	}

	sm := startup.NewStartupManager(
		k8sClient,
		k8s.GetNamespace(),
		cfg.Alert,
		&cfg.App,
	)
	if err := sm.HandleStartup(ctx); err != nil {
		klog.ErrorS(err, "failed to run startup")
		return 1
	}
	clusterID, telemetryVersion := sm.TelemetryIdentity()

	healthServer := health.NewHealthServer(cfg.HealthCheck)
	healthServer.SetFeaturePlan(featurePlan)
	securityMonitor := configureSecurityMonitor(cfg, k8sClient, now, featurePlan)
	healthServer.SetSecurityLister(securityMonitor)

	am := sm.GetAlertManager()
	am.SetClock(now)
	configureAlertManager(ctx, cfg, am)

	up := upgrader.NewUpgrader(
		&cfg.Upgrader,
		am,
		sm.GetStateManager(),
		k8s.GetDefaultClient(),
	)
	go up.CheckUpdates(ctx)

	stateMgr := sm.GetStateManager()
	stateMgr.SetClock(now)
	telemetryRun := configureTelemetryRunner(
		cfg,
		stateMgr,
		clusterID,
		telemetryVersion,
		now,
	)

	persist := configurePersistence(ctx, stateMgr, featurePlan, now)
	tracker := persist.tracker
	tracker.SetClock(now)

	graph := kwcontext.NewResourceGraph()

	auditLogger := audit.NewLogger(audit.Config{
		Enabled: cfg.AuditLog.Enabled,
		Output:  cfg.AuditLog.Output,
	})

	insightEngine := insight.NewEngine(graph, tracker)
	insightEngine.SetFeaturePlan(featurePlan)
	feedbackStore := persist.feedbackStore
	insightEngine.SetFeedbackStore(feedbackStore)
	insightEngine.SetClock(now)
	correlator := newCorrelator(
		cfg,
		persist.baseline,
		am,
		auditLogger,
		graph,
		persist.baselineCh,
		insightEngine,
		feedbackStore,
		persist.saveFeedback,
		featurePlan,
	)
	correlator.SetClock(now)
	correlator.SetFeaturePlan(featurePlan)
	insightEngine.SetActiveChecker(func(kind, namespace, name string) bool {
		for _, inc := range correlator.ActiveIncidents() {
			for _, key := range insight.IncidentGraphKeys(inc) {
				parts := strings.SplitN(key, "/", 3)
				if len(parts) == 3 && parts[0] == kind && parts[1] == namespace && parts[2] == name {
					return true
				}
			}
		}
		return false
	})

	correlator.SetAuditLogger(auditLogger)

	healthServer.SetIncidentAPI(correlator)
	healthServer.SetAlertManager(am)
	healthServer.SetDeadLetterLister(am)
	if err := healthServer.Start(ctx); err != nil {
		klog.ErrorS(err, "failed to start health check server")
		return 1
	}

	pvcMonitor := pvc.NewPvcMonitor(
		k8sClient,
		&cfg.PvcMonitor,
		correlator,
		stateMgr,
	)
	hbMonitor := heartbeat.NewHeartbeatMonitor(
		&cfg.HeartbeatMonitor,
		k8s.GetDefaultClient(),
	)

	h := handler.NewHandler(k8sClient, cfg, correlator, am)

	ctl, cleanup, err := controller.New(k8sClient, cfg, h)
	if err != nil {
		klog.ErrorS(err, "failed to create controller")
		return 1
	}
	healthServer.SetInformerLister(ctl)
	pvcMonitor.SetNamespaceFilter(ctl.NamespaceAllowed)
	pvcMonitor.SetClock(now)
	if persist.incidentSaver != nil {
		restoreIncidents(ctx, stateMgr, correlator, ctl.NamespaceAllowed)
	}
	ctl.SetTracker(tracker)
	ctl.SetGraph(graph)
	ctl.SetReadyFunc(func() { healthServer.SetReady(true) })

	statusRun := configureStatusMonitor(cfg, ctl, graph, correlator, now, featurePlan)

	metricsRun := configureMetricsMonitor(cfg, ctl, k8sClient, correlator, featurePlan)

	var tlsSweep func()
	if cfg.TlsMonitor.Enabled && featurePlan.Enabled(feature.TLSMonitoring) {
		tlsSweep = h.SweepTLSSecrets
	}

	probeRun := configureProbeRunner(cfg, correlator, k8sClient, graph, featurePlan, now)

	var kubeletRun func(context.Context)
	if cfg.KubeletTelemetryMonitor.Enabled && featurePlan.Enabled(feature.KubeletTelemetry) {
		kubeletMonitor := kubeletmetrics.New(k8sClient, cfg.KubeletTelemetryMonitor, correlator)
		kubeletMonitor.SetFeaturePlan(featurePlan)
		if cfg.KubeletTelemetryMonitor.PersistState {
			kubeletMonitor.SetStateStore(stateMgr)
		}
		healthServer.SetTelemetryLister(kubeletMonitor)
		kubeletRun = kubeletMonitor.Start
	}
	var securityRun func(context.Context)
	if featurePlan.Enabled(feature.RBACAudit) {
		securityRun = securityMonitor.Start
	}
	controlPlaneRun := configureControlPlaneMonitor(cfg, k8sClient, healthServer, correlator, now, featurePlan)

	storageRun := newStorageGraphRun(cfg, graph, correlator, ctl.NamespaceAllowed, featurePlan)
	networkRun := newNetworkGraphRun(cfg, graph, ctl.NamespaceAllowed, featurePlan)

	deps := &serverDeps{
		ctx:             ctx,
		cancel:          cancel,
		cfg:             cfg,
		healthServer:    healthServer,
		alertManager:    am,
		correlator:      correlator,
		pvcMonitor:      pvcMonitor,
		hbMonitor:       hbMonitor,
		ctl:             ctl,
		incidentCh:      persist.incidentCh,
		incidentSaver:   persist.incidentSaver,
		incidentDone:    persist.incidentDone,
		feedbackDone:    persist.feedbackDone,
		notifyStartup:   sm.NotifyStartup,
		recordAlive:     sm.RecordAlive,
		closeAudit:      auditLogger.Close,
		cleanup:         cleanup,
		tlsSweep:        tlsSweep,
		statusRun:       statusRun,
		metricsRun:      metricsRun,
		probeRun:        probeRun,
		kubeletRun:      kubeletRun,
		storageRun:      storageRun,
		networkRun:      networkRun,
		securityRun:     securityRun,
		controlPlaneRun: controlPlaneRun,
		telemetryRun:    telemetryRun,
	}
	return serve(ctx, deps)
}
