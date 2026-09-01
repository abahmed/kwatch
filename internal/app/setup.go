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
	"github.com/abahmed/kwatch/internal/feature"
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

func loadConfigAndFeaturePlan(
	now time.Time,
) (*config.Config, feature.Plan, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, feature.Plan{}, err
	}
	plan, err := buildFeaturePlan(cfg, now)
	if err != nil {
		return nil, feature.Plan{}, err
	}
	applyFeaturePlan(cfg, plan)
	return cfg, plan, nil
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
	plan feature.Plan,
	now func() time.Time,
) func(context.Context) {
	if !cfg.ActiveProbeMonitor.Enabled || !activeProbesEnabled(plan) {
		return nil
	}
	monitor := probe.New(cfg.ActiveProbeMonitor, correlator)
	monitor.SetClock(now)
	monitor.SetKubernetesClient(clientset)
	monitor.SetGraph(graph)
	monitor.SetFeaturePlan(plan)
	return monitor.Start
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
	plan feature.Plan,
	now func() time.Time,
) persistenceSetup {
	tracker := loadChangeTracker(ctx, stateMgr, plan)
	if planAllows(plan, feature.ChangePersistence) {
		go startChangeHistorySaver(ctx, stateMgr, tracker)
	}

	baseline := map[string]map[string]int64{}
	baselineCh := make(chan map[string]map[string]int64, 64)
	if planAllows(plan, feature.BaselinePersistence) {
		// Migrate before loading: the first release using the dedicated
		// ConfigMap may still have its baseline in the legacy state object.
		stateMgr.MigrateLegacyBaseline(ctx)
		baseline = stateMgr.GetBaseline(ctx)
		go startBaselineSaver(ctx, stateMgr, baselineCh, 0)
	}

	incidentDone := make(chan struct{})
	var incidentCh chan []model.PersistedIncident
	var incidentSaverForRun incidentSaver
	if planAllows(plan, feature.IncidentPersistence) {
		incidentCh = make(chan []model.PersistedIncident, 1)
		incidentSaverForRun = stateMgr
		go func() {
			defer close(incidentDone)
			startIncidentSaver(ctx, stateMgr, incidentCh)
		}()
	} else {
		close(incidentDone)
	}

	feedbackStore := loadFeedbackStore(ctx, stateMgr, plan, now)
	feedbackDone := make(chan struct{})
	feedbackCh := make(chan []insight.RCARecord, 1)
	if planAllows(plan, feature.RCAFeedback) {
		go startFeedbackSaver(ctx, stateMgr, feedbackCh, feedbackDone)
	} else {
		close(feedbackDone)
	}

	return persistenceSetup{
		tracker: tracker, baseline: baseline, baselineCh: baselineCh,
		incidentCh: incidentCh, incidentDone: incidentDone,
		incidentSaver: incidentSaverForRun, feedbackStore: feedbackStore,
		feedbackDone: feedbackDone,
		saveFeedback: feedbackSnapshotSaver(plan, feedbackCh, feedbackStore),
	}
}

func loadChangeTracker(
	ctx context.Context,
	stateMgr *state.StateManager,
	plan feature.Plan,
) *kwcontext.ChangeTracker {
	tracker := kwcontext.NewChangeTracker(0)
	if planAllows(plan, feature.ChangePersistence) {
		if changes, err := stateMgr.LoadChangeHistory(ctx); err == nil {
			tracker.Restore(changes)
		}
	}
	return tracker
}

func loadFeedbackStore(
	ctx context.Context,
	stateMgr *state.StateManager,
	plan feature.Plan,
	now func() time.Time,
) *insight.FeedbackStore {
	store := insight.NewFeedbackStore()
	store.SetClock(now)
	if planAllows(plan, feature.RCAFeedback) {
		if records, err := stateMgr.LoadRCAFeedback(ctx); err == nil {
			store.Restore(records)
		}
	}
	return store
}

func feedbackSnapshotSaver(
	plan feature.Plan,
	ch chan []insight.RCARecord,
	store *insight.FeedbackStore,
) func() {
	if !planAllows(plan, feature.RCAFeedback) {
		return nil
	}
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
	plan feature.Plan,
) *security.Monitor {
	monitor := security.NewWithConfigAndPlan(client, cfg, plan)
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
	plan feature.Plan,
) func(context.Context) {
	if !cfg.ControlPlaneMonitor.Enabled || !controlPlaneFeaturesEnabled(plan) {
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
	monitor.SetFeaturePlan(plan)
	healthServer.SetControlPlaneLister(monitor)
	return monitor.Start
}

func configureStatusMonitor(
	cfg *config.Config,
	ctl *controller.Controller,
	graph *kwcontext.ResourceGraph,
	correlator *correlation.Engine,
	now func() time.Time,
	plan feature.Plan,
) func(context.Context) {
	if !cfg.ClusterResourceMonitor.Enabled ||
		!plan.Enabled(feature.GenericStatus) {
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
	plan feature.Plan,
) func(context.Context) {
	if !cfg.RuntimeMetricsMonitor.Enabled || !plan.Enabled(feature.MetricsAPI) {
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
	plan feature.Plan,
) func(context.Context) {
	if !cfg.ClusterResourceMonitor.Enabled ||
		!plan.Enabled(feature.NetworkDetection) {
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
	plan feature.Plan,
) func(context.Context) {
	if !cfg.ClusterResourceMonitor.Enabled ||
		!plan.Enabled(feature.StorageDetection) {
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
