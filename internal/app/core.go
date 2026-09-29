package app

import (
	"context"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/core"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/knowledge/store"
	"github.com/abahmed/kwatch/internal/notice"
	"github.com/abahmed/kwatch/internal/problem"
	"github.com/abahmed/kwatch/internal/reason"
	"github.com/abahmed/kwatch/internal/signal"
	"github.com/abahmed/kwatch/internal/signal/detect"
)

// modelPruneInterval and modelRetention bound in-memory change history.
const (
	modelPruneInterval = 10 * time.Minute
	modelRetention     = 2 * time.Hour
)

// runCore runs the problem-centric core with its sources until ctx ends.
func runCore(
	ctx context.Context, deps *serverDeps, state *store.Store,
) error {
	appClock := deps.clients.Clock
	model := knowledge.NewModel(knowledge.Options{})
	var source *kube.Source
	synced := func(kind knowledge.Kind) bool {
		return source != nil && source.Synced(kind)
	}
	maintenance := deps.runtime.Policy().Maintenance()
	scope, err := filter.NewScope(
		deps.runtime.Scope(), deps.runtime.Scope().Silences(),
		maintenance.Enabled, appClock.Now)
	if err != nil {
		return err
	}
	auditCfg := deps.runtime.Lifecycle().AuditLog()
	auditLog := audit.NewLogger(audit.Config{
		Enabled: auditCfg.Enabled, Output: auditCfg.Output,
	})
	defer func() { _ = auditLog.Close() }()
	engine, err := core.NewEngine(core.Dependencies{
		Model:     model,
		Detectors: newDetectorRegistry(synced),
		Problems:  problem.NewManager(problem.Config{}, newReasoner()),
		Sink: func(_ context.Context, d problem.Decision, m notice.Message) {
			klog.V(1).InfoS("core decision", "component", "core",
				"problem", d.Problem.ID, "reason", d.Reason)
			auditLog.Record(core.AuditEntry(d, m, appClock.Now()))
			deps.deliveryManager.NotifyStory(m)
		},
		Clock: coreClock{deps.clients.Clock},
		Store: core.NewProblemStore(state),
		Investigate: core.NewInvestigator(
			kube.LogReader{Client: deps.clients.Kubernetes}.Excerpt),
		InScope: core.ProblemScope(model, scope),
		Progress: func() {
			deps.coreProgress.Touch(appClock.Now())
		},
	})
	if err != nil {
		return err
	}
	source, err = kube.NewSource(kube.SourceConfig{
		Client:      deps.clients.Kubernetes,
		Resync:      deps.runtime.Lifecycle().ResyncInterval(),
		Now:         appClock.Now,
		Submit:      engine.Submit,
		Maintenance: maintenanceAnnotations(maintenance),
	})
	if err != nil {
		return err
	}
	stats := kube.NewStatsPoller(kube.StatsConfig{
		Client: deps.clients.Kubernetes,
		Now:    appClock.Now,
		Submit: engine.Submit,
		Nodes: func() []knowledge.EntityID {
			return model.Entities(kube.KindNode)
		},
	})
	dynamicSource := kube.NewDynamicSource(kube.DynamicConfig{
		Client:    deps.clients.Dynamic,
		Discovery: deps.clients.Discovery,
		Resync:    deps.runtime.Lifecycle().ResyncInterval(),
		Now:       appClock.Now,
		Submit:    engine.Submit,
	})
	prober := kube.NewProber(kube.ProbeConfig{
		Client:   deps.clients.Kubernetes,
		Resolver: deps.clients.Resolver,
		Now:      appClock.Now,
		Submit:   engine.Submit,
	})
	runners := []func(context.Context){
		source.Run, stats.Run, dynamicSource.Run, prober.Run,
		func(ctx context.Context) { pruneModel(ctx, model, appClock.Now) },
	}
	if active, ok := newActiveProber(deps, model, engine); ok {
		runners = append(runners, active.Run)
	}
	runners = append(runners, func(ctx context.Context) {
		if !source.WaitForSync(ctx) {
			return
		}
		engine.SourcesSynced()
		deps.readiness.setCurrent("core", true)
	})
	// Sources run under a child context that ends when the engine
	// returns for any reason, and the component waits for all of them, so
	// none outlives it.
	sourceCtx, stopSources := context.WithCancel(ctx)
	var background sync.WaitGroup
	for _, run := range runners {
		background.Add(1)
		go func() {
			defer background.Done()
			run(sourceCtx)
		}()
	}
	defer background.Wait()
	defer stopSources()
	return engine.Run(ctx)
}

func newDetectorRegistry(synced func(knowledge.Kind) bool) *signal.Registry {
	return signal.NewRegistry(synced,
		detect.Container{},
		detect.NewPod(detect.PodThresholds{}),
		detect.NewNode(0),
		detect.NewWorkload(0),
		detect.Job{},
		detect.HPA{},
		detect.Claim{},
		detect.Volume{},
		detect.Service{},
		detect.Certificate{},
		detect.Missing{},
		detect.Event{},
		detect.Budget{},
		detect.Quota{},
		detect.Attachment{},
		detect.Webhook{},
		detect.NodeUsage{},
		detect.VolumeUsage{},
		detect.Custom{},
		detect.ClusterService{},
		detect.Ingress{},
		detect.EgressPolicy{},
		detect.Schedule{},
		detect.Namespace{},
		detect.ContainerResources{},
		detect.PodStorage{},
		detect.NodeHealth{},
		detect.ActiveProbe{},
	)
}

func newReasoner() *reason.Engine {
	return reason.NewEngine(0,
		reason.NodeRule{},
		reason.RolloutRule{},
		reason.ConfigRule{},
		reason.ReferenceRule{},
		reason.BackendRule{},
		reason.SchedulingRule{},
		reason.PodsRule{},
		reason.AdmissionRule{},
		reason.QuotaRule{},
		reason.NetworkPolicyRule{},
		reason.MetricsAPIRule{},
		reason.DNSRule{},
		reason.TopologyRule{},
		reason.RegistryRule{},
	)
}

func pruneModel(
	ctx context.Context, model *knowledge.Model, now func() time.Time,
) {
	ticker := time.NewTicker(modelPruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			model.Prune(now().Add(-modelRetention))
		}
	}
}

// coreClock adapts the application clock to the core's timer needs.
type coreClock struct {
	now interface{ Now() time.Time }
}

func (c coreClock) Now() time.Time { return c.now.Now() }

func (c coreClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// newActiveProber builds user-configured probes from the probe monitor
// configuration, when enabled.
func newActiveProber(
	deps *serverDeps, model knowledge.Reader, engine *core.Engine,
) (*kube.ActiveProber, bool) {
	cfg := deps.runtime.ActiveProbe()
	if !cfg.Enabled {
		return nil, false
	}
	var targets []kube.ProbeTarget
	for _, t := range cfg.HTTP {
		targets = append(targets, kube.ProbeTarget{
			Name: t.Name, URL: t.URL, ExpectedStatus: t.ExpectedStatus,
			LatencyWarningMs:  t.LatencyWarningMs,
			LatencyCriticalMs: t.LatencyCriticalMs,
		})
	}
	for _, t := range cfg.TCP {
		targets = append(targets,
			kube.ProbeTarget{Name: t.Name, Address: t.Address})
	}
	for _, t := range cfg.DNS {
		targets = append(targets, kube.ProbeTarget{Name: t.Name, Host: t.Host})
	}
	excluded := map[string]bool{}
	for _, ns := range cfg.ExcludeNamespaces {
		excluded[ns] = true
	}
	return kube.NewActiveProber(kube.ActiveProbeConfig{
		Targets: targets, AutoServices: cfg.AutoServices, Excluded: excluded,
		Interval:         time.Duration(cfg.IntervalSeconds) * time.Second,
		Timeout:          time.Duration(cfg.TimeoutSeconds) * time.Second,
		FailureThreshold: cfg.FailureThreshold,
		HTTPClient:       deps.clients.HTTP,
		Resolver:         deps.clients.Resolver,
		Model:            model,
		Now:              deps.clients.Clock.Now,
		Submit:           engine.Submit,
	}), true
}

// maintenanceAnnotations is empty when maintenance holds are disabled, so
// the source records nothing.
func maintenanceAnnotations(
	m config.MaintenanceConfig,
) kube.MaintenanceAnnotations {
	if !m.Enabled {
		return kube.MaintenanceAnnotations{}
	}
	return kube.MaintenanceAnnotations{
		On: m.Annotation, Until: m.UntilAnnotation,
	}
}
