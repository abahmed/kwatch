package app

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/client"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/controller"
	"github.com/abahmed/kwatch/internal/controlplane"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/crdwatch"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/metricsapi"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/networkgraph"
	"github.com/abahmed/kwatch/internal/probe"
	"github.com/abahmed/kwatch/internal/security"
	"github.com/abahmed/kwatch/internal/state"
	"github.com/abahmed/kwatch/internal/statuswatch"
	"github.com/abahmed/kwatch/internal/storagegraph"
)

func loadConfig() (*config.Config, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyStartupCRD(ctx context.Context, cfg *config.Config) error {
	if !cfg.CrdConfig.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		return fmt.Errorf("get rest config for startup CRD: %w", err)
	}
	return crdwatch.ApplyStartupConfig(ctx, cfg, restCfg, k8s.GetNamespace())
}

func configureProbeRunner(
	cfg *config.Config,
	correlator *correlation.Engine,
	clientset kubernetes.Interface,
	graph *kwcontext.ResourceGraph,
	now func() time.Time,
) func(context.Context) {
	if !cfg.ActiveProbeMonitor.Enabled ||
		!activeProbesEnabled(cfg.ActiveProbeMonitor) {
		return nil
	}
	monitor := probe.New(cfg.ActiveProbeMonitor, correlator)
	monitor.SetClock(now)
	monitor.SetKubernetesClient(clientset)
	monitor.SetGraph(graph)
	return monitor.Start
}

func activeProbesEnabled(cfg config.ActiveProbeMonitor) bool {
	return len(cfg.HTTP) > 0 || len(cfg.TCP) > 0 || len(cfg.DNS) > 0 ||
		cfg.AutoServices
}

type persistenceSetup struct {
	tracker       *kwcontext.ChangeTracker
	baseline      map[string]map[string]int64
	baselineCh    chan map[string]map[string]int64
	incidentCh    chan []model.PersistedIncident
	incidentDone  chan struct{}
	incidentSaver incidentSaver
	feedbackStore *insight.FeedbackStore
	feedbackDone  chan struct{}
	saveFeedback  func()
}

func configurePersistence(
	ctx context.Context,
	stateMgr *state.StateManager,
	now func() time.Time,
) persistenceSetup {
	tracker := loadChangeTracker(ctx, stateMgr)
	go startChangeHistorySaver(ctx, stateMgr, tracker)

	baselineCh := make(chan map[string]map[string]int64, 64)
	// Migrate before loading: the first release using the dedicated ConfigMap
	// may still have its baseline in the legacy state object.
	stateMgr.MigrateLegacyBaseline(ctx)
	baseline := stateMgr.GetBaseline(ctx)
	go startBaselineSaver(ctx, stateMgr, baselineCh, 0)

	incidentDone := make(chan struct{})
	incidentCh := make(chan []model.PersistedIncident, 1)
	var incidentSaverForRun incidentSaver = stateMgr
	go func() {
		defer close(incidentDone)
		startIncidentSaver(ctx, stateMgr, incidentCh)
	}()

	feedbackStore := loadFeedbackStore(ctx, stateMgr, now)
	feedbackDone := make(chan struct{})
	feedbackCh := make(chan []insight.RCARecord, 1)
	go startFeedbackSaver(ctx, stateMgr, feedbackCh, feedbackDone)

	return persistenceSetup{
		tracker: tracker, baseline: baseline, baselineCh: baselineCh,
		incidentCh: incidentCh, incidentDone: incidentDone,
		incidentSaver: incidentSaverForRun, feedbackStore: feedbackStore,
		feedbackDone: feedbackDone,
		saveFeedback: feedbackSnapshotSaver(feedbackCh, feedbackStore),
	}
}

func loadChangeTracker(
	ctx context.Context,
	stateMgr *state.StateManager,
) *kwcontext.ChangeTracker {
	tracker := kwcontext.NewChangeTracker(0)
	if changes, err := stateMgr.LoadChangeHistory(ctx); err == nil {
		tracker.Restore(changes)
	}
	return tracker
}

func loadFeedbackStore(
	ctx context.Context,
	stateMgr *state.StateManager,
	now func() time.Time,
) *insight.FeedbackStore {
	store := insight.NewFeedbackStore()
	store.SetClock(now)
	if records, err := stateMgr.LoadRCAFeedback(ctx); err == nil {
		store.Restore(records)
	}
	return store
}

func feedbackSnapshotSaver(
	ch chan []insight.RCARecord,
	store *insight.FeedbackStore,
) func() {
	return func() { trySendFeedbackSnapshot(ch, store.Snapshot()) }
}

func startChangeHistorySaver(
	ctx context.Context,
	stateMgr *state.StateManager,
	tracker *kwcontext.ChangeTracker,
) {
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

func configureSecurityMonitor(
	cfg *config.Config,
	client kubernetes.Interface,
	now func() time.Time,
) *security.Monitor {
	monitor := security.NewWithConfig(client, cfg)
	monitor.SetClock(now)
	namespaces := cfg.AllowedNamespaces
	if len(namespaces) == 0 {
		namespaces = []string{k8s.GetNamespace()}
	}
	monitor.SetNamespaces(namespaces)
	return monitor
}

func configureControlPlaneMonitor(
	cfg *config.Config,
	clientset kubernetes.Interface,
	healthServer *health.HealthServer,
	correlator *correlation.Engine,
	now func() time.Time,
) func(context.Context) {
	if !cfg.ControlPlaneMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create control-plane monitor")
		return nil
	}
	monitor, err := controlplane.New(
		restCfg, clientset, cfg.ControlPlaneMonitor, correlator,
	)
	if err != nil {
		klog.ErrorS(err, "failed to initialize control-plane monitor")
		return nil
	}
	monitor.SetClock(now)
	healthServer.SetControlPlaneLister(monitor)
	return monitor.Start
}

func configureStatusMonitor(
	cfg *config.Config,
	ctl *controller.Controller,
	graph *kwcontext.ResourceGraph,
	correlator *correlation.Engine,
	now func() time.Time,
) func(context.Context) {
	if !cfg.ClusterResourceMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create generic status monitor")
		return nil
	}
	monitor, err := statuswatch.New(
		restCfg, correlator,
		time.Duration(cfg.ResyncSeconds)*time.Second,
	)
	if err != nil {
		klog.ErrorS(err, "failed to initialize generic status monitor")
		return nil
	}
	monitor.SetClock(now)
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

func configureMetricsMonitor(
	cfg *config.Config,
	ctl *controller.Controller,
	clientset kubernetes.Interface,
	correlator *correlation.Engine,
) func(context.Context) {
	if !cfg.RuntimeMetricsMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create runtime metrics monitor")
		return nil
	}
	monitor, err := metricsapi.New(
		restCfg, clientset, cfg.RuntimeMetricsMonitor, correlator,
	)
	if err != nil {
		klog.ErrorS(err, "failed to initialize runtime metrics monitor")
		return nil
	}
	monitor.SetNamespaceFilter(ctl.NamespaceAllowed)
	return monitor.Start
}

func newNetworkGraphRun(
	cfg *config.Config,
	graph *kwcontext.ResourceGraph,
	namespaceAllowed func(string) bool,
) func(context.Context) {
	if !cfg.ClusterResourceMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create rest config for network graph monitor")
		return nil
	}
	monitor, err := networkgraph.New(
		restCfg, graph, time.Duration(cfg.ResyncSeconds)*time.Second,
	)
	if err != nil {
		klog.ErrorS(err, "failed to initialize network graph monitor")
		return nil
	}
	monitor.SetNamespaceFilter(namespaceAllowed)
	return func(ctx context.Context) {
		if err := monitor.Start(ctx); err != nil {
			klog.ErrorS(err, "network graph monitor stopped")
		}
	}
}

func newStorageGraphRun(
	cfg *config.Config,
	graph *kwcontext.ResourceGraph,
	correlator *correlation.Engine,
	namespaceAllowed func(string) bool,
) func(context.Context) {
	if !cfg.ClusterResourceMonitor.Enabled {
		return nil
	}
	restCfg, err := client.GetRestConfig(&cfg.App)
	if err != nil {
		klog.ErrorS(err, "failed to create rest config for storage graph monitor")
		return nil
	}
	monitor, err := storagegraph.New(
		restCfg, graph, time.Duration(cfg.ResyncSeconds)*time.Second,
	)
	if err != nil {
		klog.ErrorS(err, "failed to initialize storage graph monitor")
		return nil
	}
	monitor.SetCorrelator(correlator)
	monitor.SetNamespaceFilter(namespaceAllowed)
	return func(ctx context.Context) {
		if err := monitor.Start(ctx); err != nil {
			klog.ErrorS(err, "storage graph monitor stopped")
		}
	}
}
