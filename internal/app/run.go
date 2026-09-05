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
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/kubeletmetrics"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/upgrader"
	"github.com/abahmed/kwatch/internal/version"
)

// serverDeps bundles the wired components so the background loop and the
// shutdown sequence can live in their own functions.
type serverDeps struct {
	ctx            context.Context
	cancel         context.CancelFunc
	cfg            *config.Config
	healthServer   *health.HealthServer
	alertManager   *alert.AlertManager
	correlator     *correlation.Engine
	pvcMonitor     *pvc.PvcMonitor
	hbMonitor      *heartbeat.HeartbeatMonitor
	ctl            *controller.Controller
	incidentCh     chan []model.PersistedIncident
	incidentSaver  incidentSaver
	incidentDone   <-chan struct{}
	feedbackDone   <-chan struct{}
	initialized    <-chan struct{}
	controllerDone chan struct{}
	notifyStartup  func()
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

	cfg, err := loadConfig()
	if err != nil {
		klog.ErrorS(err, "failed to load config")
		return 1
	}
	cfg.WatchStartTime = now()

	klog.InfoS(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	k8s.InitHTTPClient(&cfg.App)
	if err := applyStartupCRD(ctx, cfg); err != nil {
		klog.ErrorS(err, "failed to apply startup CRD configuration")
		return 1
	}
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
	securityMonitor := configureSecurityMonitor(cfg, k8sClient, now)
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

	persist := configurePersistence(ctx, stateMgr, now)
	tracker := persist.tracker
	tracker.SetClock(now)

	graph := kwcontext.NewResourceGraph()

	auditLogger := audit.NewLogger(audit.Config{
		Enabled: cfg.AuditLog.Enabled,
		Output:  cfg.AuditLog.Output,
	})

	insightEngine := insight.NewEngine(graph, tracker)
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
	)
	correlator.SetClock(now)
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
		if closeErr := auditLogger.Close(); closeErr != nil {
			klog.ErrorS(closeErr, "failed to close audit logger")
		}
		klog.ErrorS(err, "failed to create controller")
		return 1
	}
	healthServer.SetInformerLister(ctl)
	namespaces, watchAll := ctl.NamespaceScope()
	securityMonitor.SetNamespaces(namespaces)
	securityMonitor.SetAllNamespaces(watchAll)
	pvcMonitor.SetNamespaceScope(namespaces, cfg.ForbiddenNamespaces, watchAll)
	pvcMonitor.SetNamespaceFilter(ctl.NamespaceAllowed)
	pvcMonitor.SetClock(now)
	if persist.incidentSaver != nil {
		restoreIncidents(ctx, stateMgr, correlator, ctl.NamespaceAllowed)
	}
	ctl.SetTracker(tracker)
	ctl.SetGraph(graph)
	initialized := make(chan struct{})
	ctl.SetReadyFunc(func() {
		healthServer.SetReady(true)
		close(initialized)
	})
	if err := healthServer.Start(ctx); err != nil {
		cleanup()
		if err := auditLogger.Close(); err != nil {
			klog.ErrorS(err, "failed to close audit logger")
		}
		klog.ErrorS(err, "failed to start health check server")
		return 1
	}

	statusRun := configureStatusMonitor(
		cfg, ctl, graph, correlator, healthServer, now,
	)

	metricsRun := configureMetricsMonitor(
		cfg, ctl, k8sClient, correlator, healthServer,
	)

	var tlsSweep func()
	if cfg.TlsMonitor.Enabled {
		tlsSweep = h.SweepTLSSecrets
	}

	probeRun := configureProbeRunner(
		cfg, ctl, correlator, k8sClient, graph, now,
	)

	var kubeletRun func(context.Context)
	if cfg.KubeletTelemetryMonitor.Enabled {
		kubeletMonitor := kubeletmetrics.New(k8sClient, cfg.KubeletTelemetryMonitor, correlator)
		kubeletMonitor.SetNamespaceScope(namespaces, watchAll)
		kubeletMonitor.SetNamespaceFilter(ctl.NamespaceAllowed)
		kubeletMonitor.SetClock(now)
		if cfg.KubeletTelemetryMonitor.PersistState {
			kubeletMonitor.SetStateStore(stateMgr)
		}
		healthServer.SetTelemetryLister(kubeletMonitor)
		kubeletRun = kubeletMonitor.Start
	}
	var securityRun func(context.Context)
	securityRun = securityMonitor.Start
	controlPlaneRun := configureControlPlaneMonitor(
		cfg, k8sClient, healthServer, correlator, now,
	)

	storageRun := newStorageGraphRun(
		cfg, graph, correlator, ctl, healthServer,
	)
	networkRun := newNetworkGraphRun(cfg, graph, ctl, healthServer)

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
		initialized:     initialized,
		controllerDone:  make(chan struct{}),
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
