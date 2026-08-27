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
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/k8s"
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
	notifyStartup func()
	// recordAlive stamps the liveness marker that lets the next start report
	// how long monitoring was down.
	recordAlive func(context.Context)
	cleanup     func()
	tlsSweep    func()
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

	k8s.InitHTTPClient(&cfg.App)
	k8sClient := client.Create(&cfg.App)

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
	baseline := stateMgr.GetBaseline(ctx)
	stateMgr.MigrateLegacyBaseline(ctx)

	baselineCh := make(chan map[string]map[string]int64, 64)
	go startBaselineSaver(ctx, stateMgr, baselineCh, 0)

	incidentCh := make(chan []model.PersistedIncident, 1)
	go startIncidentSaver(ctx, stateMgr, incidentCh)

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
	restoreIncidents(ctx, stateMgr, correlator)

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
		am,
		correlator,
		stateMgr,
	)
	hbMonitor := heartbeat.NewHeartbeatMonitor(&cfg.HeartbeatMonitor)

	h := handler.NewHandler(k8sClient, cfg, correlator, am)

	ctl, cleanup := controller.New(k8sClient, cfg, h)
	ctl.SetTracker(tracker)
	ctl.SetGraph(graph)
	ctl.SetReadyFunc(func() { healthServer.SetReady(true) })

	var tlsSweep func()
	if cfg.TlsMonitor.Enabled {
		tlsSweep = h.SweepTLSSecrets
	}

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
		notifyStartup: sm.NotifyStartup,
		recordAlive:   sm.RecordAlive,
		cleanup:       cleanup,
		tlsSweep:      tlsSweep,
	}
	return serve(ctx, deps)
}
