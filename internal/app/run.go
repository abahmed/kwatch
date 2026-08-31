package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/controlplane"
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
	"github.com/abahmed/kwatch/internal/security"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/state"
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
	securityMonitor := configureSecurityMonitor(cfg, k8sClient)
	healthServer.SetSecurityLister(securityMonitor)

	am := sm.GetAlertManager()
	configureAlertManager(ctx, cfg, am)

	up := upgrader.NewUpgrader(&cfg.Upgrader, am, sm.GetStateManager())
	go up.CheckUpdates(ctx)

	stateMgr := sm.GetStateManager()

	tracker := loadChangeTracker(ctx, stateMgr)
	go startChangeHistorySaver(ctx, stateMgr, tracker)
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
	hbMonitor := heartbeat.NewHeartbeatMonitor(&cfg.HeartbeatMonitor)

	h := handler.NewHandler(k8sClient, cfg, correlator, am)

	ctl, cleanup, err := controller.New(k8sClient, cfg, h)
	if err != nil {
		klog.ErrorS(err, "failed to create controller")
		return 1
	}
	healthServer.SetInformerLister(ctl)
	pvcMonitor.SetNamespaceFilter(ctl.NamespaceAllowed)
	restoreIncidents(ctx, stateMgr, correlator, ctl.NamespaceAllowed)
	ctl.SetTracker(tracker)
	ctl.SetGraph(graph)
	ctl.SetReadyFunc(func() { healthServer.SetReady(true) })

	statusRun := configureStatusMonitor(cfg, ctl, graph, correlator)

	metricsRun := configureMetricsMonitor(cfg, ctl, k8sClient, correlator)

	var tlsSweep func()
	if cfg.TlsMonitor.Enabled {
		tlsSweep = h.SweepTLSSecrets
	}

	var probeRun func(context.Context)
	if cfg.ActiveProbeMonitor.Enabled {
		probeMonitor := probe.New(cfg.ActiveProbeMonitor, correlator)
		probeMonitor.SetKubernetesClient(k8sClient)
		probeMonitor.SetGraph(graph)
		probeRun = probeMonitor.Start
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
	securityRun := securityMonitor.Start
	controlPlaneRun := configureControlPlaneMonitor(cfg, k8sClient, healthServer, correlator)

	storageRun := newStorageGraphRun(cfg, graph, correlator)
	networkRun := newNetworkGraphRun(cfg, graph)

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
		incidentCh:      incidentCh,
		incidentSaver:   stateMgr,
		incidentDone:    incidentDone,
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
	}
	return serve(ctx, deps)
}

func loadChangeTracker(ctx context.Context, stateMgr *state.StateManager) *kwcontext.ChangeTracker {
	tracker := kwcontext.NewChangeTracker(0)
	if changes, err := stateMgr.LoadChangeHistory(ctx); err == nil {
		tracker.Restore(changes)
	}
	return tracker
}

func startChangeHistorySaver(ctx context.Context, stateMgr *state.StateManager, tracker *kwcontext.ChangeTracker) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := stateMgr.SaveChangeHistory(ctx, tracker.Snapshot()); err != nil {
				klog.ErrorS(err, "failed to persist recent change history")
			}
		}
	}
}

func configureSecurityMonitor(cfg *config.Config, client kubernetes.Interface) *security.Monitor {
	monitor := security.New(client)
	namespaces := cfg.AllowedNamespaces
	if len(namespaces) == 0 {
		namespaces = []string{k8s.GetNamespace()}
	}
	monitor.SetNamespaces(namespaces)
	return monitor
}

func configureControlPlaneMonitor(cfg *config.Config, clientset kubernetes.Interface, healthServer *health.HealthServer, correlator *correlation.Engine) func(context.Context) {
	if !cfg.ControlPlaneMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create control-plane monitor")
		return nil
	}
	monitor, err := controlplane.New(restCfg, clientset, cfg.ControlPlaneMonitor, correlator)
	if err != nil {
		klog.ErrorS(err, "failed to initialize control-plane monitor")
		return nil
	}
	healthServer.SetControlPlaneLister(monitor)
	return monitor.Start
}

func configureStatusMonitor(cfg *config.Config, ctl *controller.Controller, graph *kwcontext.ResourceGraph, correlator *correlation.Engine) func(context.Context) {
	if !cfg.ClusterResourceMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create generic status monitor")
		return nil
	}
	monitor, err := statuswatch.New(restCfg, correlator, time.Duration(cfg.ResyncSeconds)*time.Second)
	if err != nil {
		klog.ErrorS(err, "failed to initialize generic status monitor")
		return nil
	}
	monitor.SetNamespaceFilter(ctl.NamespaceAllowed)
	monitor.SetConditionRules(cfg.CrdConfig.FailureConditions)
	monitor.SetGraphReferenceRules(cfg.CrdConfig.GraphReferences)
	monitor.SetGraph(graph)
	return func(ctx context.Context) {
		if err := monitor.Start(ctx); err != nil {
			klog.ErrorS(err, "generic status monitor stopped")
		}
	}
}

func configureMetricsMonitor(cfg *config.Config, ctl *controller.Controller, clientset kubernetes.Interface, correlator *correlation.Engine) func(context.Context) {
	if !cfg.RuntimeMetricsMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create runtime metrics monitor")
		return nil
	}
	monitor, err := metricsapi.New(restCfg, clientset, cfg.RuntimeMetricsMonitor, correlator)
	if err != nil {
		klog.ErrorS(err, "failed to initialize runtime metrics monitor")
		return nil
	}
	monitor.SetNamespaceFilter(ctl.NamespaceAllowed)
	return monitor.Start
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

func newStorageGraphRun(cfg *config.Config, graph *kwcontext.ResourceGraph, correlator *correlation.Engine) func(context.Context) {
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
	monitor.SetCorrelator(correlator)
	return func(ctx context.Context) {
		if err := monitor.Start(ctx); err != nil {
			klog.ErrorS(err, "storage graph monitor stopped")
		}
	}
}
