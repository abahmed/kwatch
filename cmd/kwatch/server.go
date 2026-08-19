package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
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
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/handler"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/heartbeat"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/pvc"
	"github.com/abahmed/kwatch/internal/startup"
	"github.com/abahmed/kwatch/internal/state"
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
	cleanup       func()
	tlsSweep      func()
}

// runServer loads config and wires all monitors, then runs until shutdown.
func runServer() int {
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

	correlator := newCorrelator(cfg, baseline, am, auditLogger, graph, baselineCh)
	restoreIncidents(ctx, stateMgr, correlator)

	correlator.SetAuditLogger(auditLogger)

	healthServer.SetIncidentAPI(correlator)
	healthServer.SetAlertManager(am)
	healthServer.SetDeadLetterLister(am)
	if err := healthServer.Start(ctx); err != nil {
		klog.ErrorS(err, "failed to start health check server")
		return 1
	}

	pvcMonitor := pvc.NewPvcMonitor(k8sClient, &cfg.PvcMonitor, am, correlator, stateMgr)
	hbMonitor := heartbeat.NewHeartbeatMonitor(&cfg.HeartbeatMonitor)

	h := handler.NewHandler(k8sClient, cfg, correlator, am)
	insightEngine := insight.NewEngine(graph, tracker)
	h.SetInsightEngine(insightEngine)

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
		cleanup:       cleanup,
		tlsSweep:      tlsSweep,
	}
	return serve(ctx, deps)
}

// configureAlertManager applies silences, templates, and starts delivery.
func configureAlertManager(ctx context.Context, cfg *config.Config, am *alert.AlertManager) {
	am.SetSilences(cfg.Silences)
	am.SetTemplates(cfg.Templates)
	if cfg.MaxRecentLogLines > 0 {
		am.SetMaxLogLines(int(cfg.MaxRecentLogLines))
	}
	am.Start(ctx)
}

// engineHolder carries the engine through hook registration so the hooks can
// reference it after construction completes.
type engineHolder struct {
	engine *correlation.Engine
}

// newCorrelator builds the correlation engine with its incident lifecycle,
// mass-failure, and baseline hooks wired to the alerting stack.
func newCorrelator(
	cfg *config.Config, baseline map[string]map[string]int64,
	am *alert.AlertManager, auditLogger *audit.AuditLogger,
	graph *kwcontext.ResourceGraph, baselineCh chan<- map[string]map[string]int64,
) *correlation.Engine {
	holder := &engineHolder{}
	opts := &engineOptions{
		cfg:          cfg,
		baseline:     baseline,
		alertManager: am,
		auditLogger:  auditLogger,
		graph:        graph,
		baselineCh:   baselineCh,
	}

	holder.engine = correlation.NewEngine(correlation.Config{
		Window:                     time.Duration(cfg.Correlation.Window) * time.Minute,
		LifecycleInterval:          time.Duration(cfg.Correlation.LifecycleInterval) * time.Minute,
		Baseline:                   baseline,
		Enricher:                   &enricher.DefaultEnricher{SeverityByOwnerKind: cfg.SeverityByOwnerKind, SeverityByReason: cfg.SeverityByReason},
		EscalationEnabled:          cfg.Correlation.Escalation.Enabled,
		EscalationTiers:            cfg.Correlation.Escalation.Tiers,
		InhibitNodeSuppressesPods:  cfg.Inhibition.NodeSuppressesPods,
		RenotifyIntervalBySeverity: renotifyIntervalBySeverity(cfg.Correlation.Renotify.IntervalBySeverity),
		RenotifyMaxPerIncident:     cfg.Correlation.Renotify.MaxPerIncident,
		Runbooks:                   cfg.Runbooks,
		ResolveHoldDown:            time.Duration(cfg.Correlation.ResolveHoldDown) * time.Second,
		MaxBaseline:                cfg.Correlation.MaxBaseline,
		SmartGroupingWindow:        time.Duration(cfg.SmartGrouping.WindowSeconds) * time.Second,
		LifecycleHook:              lifecycleHook(opts, holder),
		MassFailureHook:            massFailureHook(opts, holder),
		OnBaselineChange:           onBaselineChange(baselineCh),
	})
	return holder.engine
}

// engineOptions groups the inputs shared by the correlator hooks.
type engineOptions struct {
	cfg          *config.Config
	baseline     map[string]map[string]int64
	alertManager *alert.AlertManager
	auditLogger  *audit.AuditLogger
	graph        *kwcontext.ResourceGraph
	baselineCh   chan<- map[string]map[string]int64
}

// lifecycleHook audits and notifies each incident edge unless it was skipped.
func lifecycleHook(opts *engineOptions, holder *engineHolder) func(*model.Incident, model.IncidentAction) {
	return func(inc *model.Incident, action model.IncidentAction) {
		if action != model.ActionSkip {
			opts.auditLogger.LogIncident(inc, action)
			opts.alertManager.NotifyIncident(inc, action, nil)
		}
		metrics.Default.ActiveIncidents.Store(int64(holder.engine.ActiveCount()))
	}
}

// massFailureHook reports new mass failures and resolves ones that cleared.
func massFailureHook(opts *engineOptions, holder *engineHolder) func() {
	return func() {
		allIncidents := holder.engine.SnapshotAll()
		incList := make([]*model.Incident, 0, len(allIncidents))
		for _, inc := range allIncidents {
			incList = append(incList, inc)
		}
		mfs := insight.ScanMassFailures(incList, opts.graph)

		current := make(map[string]insight.MassFailure, len(mfs))
		for _, mf := range mfs {
			current[mf.SharedDependency] = mf
		}
		notifyNewMassFailures(opts, holder, current)
		resolveClearedMassFailures(opts, holder, current)
	}
}

// notifyNewMassFailures fires active incidents for mass failures the engine is
// not yet tracking.
func notifyNewMassFailures(opts *engineOptions, holder *engineHolder, current map[string]insight.MassFailure) {
	for key, mf := range current {
		incKey := correlation.MassFailureKey(key)
		if holder.engine.HasMassFailure(incKey) {
			continue
		}
		klog.V(2).InfoS("mass failure detected", "message", mf.Describe())
		inc := &model.Incident{
			Key:       incKey,
			Reason:    mf.Reason,
			Namespace: mf.Namespace,
			Resource:  mf.ResourceKind,
			Name:      key,
			State:     model.StateActive,
		}
		holder.engine.AddMassFailure(inc)
		opts.alertManager.NotifyIncident(inc, model.ActionCreate, nil)
	}
}

// resolveClearedMassFailures resolves tracked mass failures whose shared
// dependency no longer appears in the failure set.
func resolveClearedMassFailures(opts *engineOptions, holder *engineHolder, current map[string]insight.MassFailure) {
	tracked := holder.engine.MassFailureSet()
	for incKey, inc := range tracked {
		dep := strings.TrimPrefix(string(incKey), "mass-failure/")
		if _, exists := current[dep]; exists {
			continue
		}
		klog.V(2).InfoS("mass failure resolved", "dependency", dep)
		resolved := inc.Clone()
		resolved.State = model.StateResolved
		holder.engine.RemoveMassFailure(incKey)
		opts.alertManager.NotifyIncident(resolved, model.ActionResolved, nil)
	}
}

// onBaselineChange forwards baseline snapshots to the persistent saver.
func onBaselineChange(baselineCh chan<- map[string]map[string]int64) func(map[string]map[string]int64) {
	return func(b map[string]map[string]int64) {
		total := 0
		for _, pods := range b {
			total += len(pods)
		}
		metrics.Default.BaselineSize.Store(int64(total))
		select {
		case baselineCh <- b:
		default:
			// Channel full: drop oldest, keep newest
		}
	}
}

// restoreIncidents loads previously persisted incidents back into memory.
func restoreIncidents(ctx context.Context, stateMgr *state.StateManager, correlator *correlation.Engine) {
	var persisted []model.PersistedIncident
	if err := stateMgr.GetIncidents(ctx, &persisted); err != nil {
		klog.ErrorS(err, "failed to restore incidents from configmap")
		return
	}
	if len(persisted) == 0 {
		return
	}
	restored := make(map[model.IncidentKey]*model.Incident, len(persisted))
	for i := range persisted {
		inc := persisted[i].ToIncident()
		restored[inc.Key] = inc
	}
	correlator.RestoreIncidents(restored)
	klog.InfoS("restored incidents from configmap", "count", len(persisted))
}

// serve starts the controller loop and background monitors, then waits for
// shutdown, returning the process exit code.
func serve(ctx context.Context, deps *serverDeps) int {
	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	wg.Add(4)
	if deps.tlsSweep != nil {
		wg.Add(1)
	}

	go func() {
		defer wg.Done()
		deps.correlator.StartCleanup(ctx)
	}()
	go func() {
		defer wg.Done()
		deps.pvcMonitor.Start(ctx)
	}()
	go func() {
		defer wg.Done()
		deps.hbMonitor.Start(ctx)
	}()
	go func() {
		defer wg.Done()
		interval := 60 * time.Second
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				if snap := deps.correlator.SnapshotPersisted(); len(snap) > 0 {
					trySendIncidentSnapshot(deps.incidentCh, snap)
				}
				return
			case <-ticker.C:
				if snap := deps.correlator.SnapshotPersisted(); len(snap) > 0 {
					trySendIncidentSnapshot(deps.incidentCh, snap)
				}
			}
		}
	}()
	if deps.tlsSweep != nil {
		go func() {
			defer wg.Done()
			deps.tlsSweep()
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					deps.tlsSweep()
				}
			}
		}()
	}
	if deps.cfg.CrdConfig.Enabled {
		startCRDWatcher(ctx, deps)
	}

	go func() {
		deps.notifyStartup()

		workers := deps.cfg.Workers
		if workers < 1 {
			workers = 1
		}
		if err := deps.ctl.Run(ctx, workers); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	return waitShutdown(deps, &wg, errCh)
}

// startCRDWatcher launches the CRD watcher against the cluster rest config.
func startCRDWatcher(ctx context.Context, deps *serverDeps) {
	restCfg, err := client.GetRestConfig(&deps.cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to get rest config for CRD watcher")
		return
	}
	resync := time.Duration(deps.cfg.ResyncSeconds) * time.Second
	w := crdwatch.New(deps.cfg, deps.alertManager, deps.correlator, restCfg, k8s.GetNamespace(), resync)
	if err := w.Start(ctx); err != nil {
		klog.ErrorS(err, "CRD watcher error")
	}
}

// waitShutdown blocks until a signal or controller failure, then drains.
func waitShutdown(deps *serverDeps, wg *sync.WaitGroup, errCh <-chan error) int {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case <-sigCh:
		klog.InfoS("shutting down gracefully...")
	case err := <-errCh:
		if err != nil {
			klog.ErrorS(err, "controller startup failed, shutting down")
		}
	}
	deps.cancel()

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(10 * time.Second):
		klog.InfoS("timed out waiting for background tasks")
	}

	select {
	case <-deps.alertManager.Done():
	case <-time.After(10 * time.Second):
		klog.InfoS("timed out waiting for alert manager to drain")
	}
	shutdownCtx, sc := context.WithTimeout(context.Background(), 10*time.Second)
	deps.healthServer.SetReady(false)
	if err := deps.healthServer.Stop(shutdownCtx); err != nil {
		klog.ErrorS(err, "failed to stop health check server")
	}
	sc()
	deps.cleanup()
	return 0
}
