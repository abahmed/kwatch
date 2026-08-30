package app

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/crdwatch"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/kubeletmetrics"
	"github.com/abahmed/kwatch/internal/metricsapi"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/networkgraph"
	"github.com/abahmed/kwatch/internal/probe"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/statuswatch"
	"github.com/abahmed/kwatch/internal/storagegraph"
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
	notifyStartup func()
	// recordAlive stamps the liveness marker that lets the next start report
	// how long monitoring was down.
	recordAlive func(context.Context)
	closeAudit  func() error
	cleanup     func()
	tlsSweep    func()
	statusRun   func(context.Context)
	metricsRun  func(context.Context)
	probeRun    func(context.Context)
	kubeletRun  func(context.Context)
	storageRun  func(context.Context)
	networkRun  func(context.Context)
}

// Run loads config and wires all monitors, then runs until shutdown.
func Run() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig()
	if err != nil {
		klog.ErrorS(err, "failed to load config")
		return 1
	}
	cfg.WatchStartTime = time.Now()

	klog.InfoS(fmt.Sprintf(constant.WelcomeMsg, version.Short()))

	if cfg.CrdConfig.Enabled {
		restCfg, err := client.GetRestConfig(&cfg.App)
		if err != nil {
			klog.ErrorS(err, "failed to get rest config for startup CRD")
			return 1
		}
		if err := crdwatch.ApplyStartupConfig(ctx, cfg, restCfg, k8s.GetNamespace()); err != nil {
			klog.ErrorS(err, "failed to apply startup CRD configuration")
			return 1
		}
	}
	k8s.InitHTTPClient(&cfg.App)
	k8sClient, err := client.Create(&cfg.App)
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

	healthServer := health.NewHealthServer(cfg.HealthCheck)

	am := sm.GetAlertManager()
	configureAlertManager(ctx, cfg, am)

	up := upgrader.NewUpgrader(&cfg.Upgrader, am, sm.GetStateManager())
	go up.CheckUpdates(ctx)

	stateMgr := sm.GetStateManager()
	stateMgr.MigrateLegacyBaseline(ctx)
	// Migrate before loading: on the first release that uses the dedicated
	// ConfigMap, the only baseline may still be in kwatch-state. Loading first
	// would construct the engine with an empty baseline and re-alert every
	// pre-existing incident during this process lifetime.
	baseline := stateMgr.GetBaseline(ctx)

	baselineCh := make(chan map[string]map[string]int64, 64)
	go startBaselineSaver(ctx, stateMgr, baselineCh, 0)

	incidentCh := make(chan []model.PersistedIncident, 1)
	incidentDone := make(chan struct{})
	go func() {
		defer close(incidentDone)
		startIncidentSaver(ctx, stateMgr, incidentCh)
	}()

	tracker := kwcontext.NewChangeTracker(0)
	graph := kwcontext.NewResourceGraph()

	auditLogger := audit.NewLogger(audit.Config{
		Enabled: cfg.AuditLog.Enabled,
		Output:  cfg.AuditLog.Output,
	})

	insightEngine := insight.NewEngine(graph, tracker)
	correlator := newCorrelator(
		cfg,
		baseline,
		am,
		auditLogger,
		graph,
		baselineCh,
		insightEngine,
	)

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
	hbMonitor := heartbeat.NewHeartbeatMonitor(&cfg.HeartbeatMonitor)

	h := handler.NewHandler(k8sClient, cfg, correlator, am)

	ctl, cleanup, err := controller.New(k8sClient, cfg, h)
	if err != nil {
		klog.ErrorS(err, "failed to create controller")
		return 1
	}
	pvcMonitor.SetNamespaceFilter(ctl.NamespaceAllowed)
	restoreIncidents(ctx, stateMgr, correlator, ctl.NamespaceAllowed)
	ctl.SetTracker(tracker)
	ctl.SetGraph(graph)
	ctl.SetReadyFunc(func() { healthServer.SetReady(true) })

	var statusRun func(context.Context)
	if cfg.ClusterResourceMonitor.Enabled {
		if restCfg, restErr := client.GetRestConfig(&cfg.App); restErr != nil {
			klog.ErrorS(restErr, "failed to create generic status monitor")
		} else if monitor, monitorErr := statuswatch.New(
			restCfg, correlator, time.Duration(cfg.ResyncSeconds)*time.Second,
		); monitorErr != nil {
			klog.ErrorS(monitorErr, "failed to initialize generic status monitor")
		} else {
			monitor.SetNamespaceFilter(ctl.NamespaceAllowed)
			monitor.SetConditionRules(cfg.CrdConfig.FailureConditions)
			monitor.SetGraph(graph)
			statusRun = func(runCtx context.Context) {
				if err := monitor.Start(runCtx); err != nil {
					klog.ErrorS(err, "generic status monitor stopped")
				}
			}
		}
	}

	var metricsRun func(context.Context)
	if cfg.RuntimeMetricsMonitor.Enabled {
		if restCfg, restErr := client.GetRestConfig(&cfg.App); restErr != nil {
			klog.ErrorS(restErr, "failed to create runtime metrics monitor")
		} else if monitor, monitorErr := metricsapi.New(
			restCfg, k8sClient, cfg.RuntimeMetricsMonitor, correlator,
		); monitorErr != nil {
			klog.ErrorS(monitorErr, "failed to initialize runtime metrics monitor")
		} else {
			monitor.SetNamespaceFilter(ctl.NamespaceAllowed)
			metricsRun = monitor.Start
		}
	}

	var tlsSweep func()
	if cfg.TlsMonitor.Enabled {
		tlsSweep = h.SweepTLSSecrets
	}

	var probeRun func(context.Context)
	if cfg.ActiveProbeMonitor.Enabled {
		probeRun = probe.New(cfg.ActiveProbeMonitor, correlator).Start
	}

	var kubeletRun func(context.Context)
	if cfg.KubeletTelemetryMonitor.Enabled {
		kubeletMonitor := kubeletmetrics.New(k8sClient, cfg.KubeletTelemetryMonitor, correlator)
		if cfg.KubeletTelemetryMonitor.PersistState {
			kubeletMonitor.SetStateStore(stateMgr)
		}
		healthServer.SetTelemetryLister(kubeletMonitor)
		kubeletRun = kubeletMonitor.Start
	}

	storageRun := newStorageGraphRun(cfg, graph)
	networkRun := newNetworkGraphRun(cfg, graph)

	deps := &serverDeps{
		ctx:           ctx,
		cancel:        cancel,
		cfg:           cfg,
		healthServer:  healthServer,
		alertManager:  am,
		correlator:    correlator,
		pvcMonitor:    pvcMonitor,
		hbMonitor:     hbMonitor,
		ctl:           ctl,
		incidentCh:    incidentCh,
		incidentSaver: stateMgr,
		incidentDone:  incidentDone,
		notifyStartup: sm.NotifyStartup,
		recordAlive:   sm.RecordAlive,
		closeAudit:    auditLogger.Close,
		cleanup:       cleanup,
		tlsSweep:      tlsSweep,
		statusRun:     statusRun,
		metricsRun:    metricsRun,
		probeRun:      probeRun,
		kubeletRun:    kubeletRun,
		storageRun:    storageRun,
		networkRun:    networkRun,
	}
	return serve(ctx, deps)
}

func newNetworkGraphRun(cfg *config.Config, graph *kwcontext.ResourceGraph) func(context.Context) {
	if !cfg.ClusterResourceMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create rest config for network graph monitor")
		return nil
	}
	monitor, err := networkgraph.New(restCfg, graph, time.Duration(cfg.ResyncSeconds)*time.Second)
	if err != nil {
		klog.ErrorS(err, "failed to initialize network graph monitor")
		return nil
	}
	return func(ctx context.Context) {
		if err := monitor.Start(ctx); err != nil {
			klog.ErrorS(err, "network graph monitor stopped")
		}
	}
}

func newStorageGraphRun(cfg *config.Config, graph *kwcontext.ResourceGraph) func(context.Context) {
	if !cfg.ClusterResourceMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create rest config for storage graph monitor")
		return nil
	}
	monitor, err := storagegraph.New(restCfg, graph, time.Duration(cfg.ResyncSeconds)*time.Second)
	if err != nil {
		klog.ErrorS(err, "failed to initialize storage graph monitor")
		return nil
	}
	return func(ctx context.Context) {
		if err := monitor.Start(ctx); err != nil {
			klog.ErrorS(err, "storage graph monitor stopped")
		}
	}
}
