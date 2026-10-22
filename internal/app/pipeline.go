package app

import (
	"context"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/kubeclient"
	"github.com/abahmed/kwatch/internal/safego"
	"github.com/abahmed/kwatch/internal/storage"
)

// modelPruneInterval and modelRetention bound in-memory change history.
const (
	modelPruneInterval = 10 * time.Minute
	modelRetention     = 2 * time.Hour
)

// runPipeline runs the incident-centric pipeline with its sources until ctx
// ends.
func runPipeline(
	ctx context.Context, deps *serverDeps, state *storage.Store,
	guard *storeGuard, gap gapNotice,
) error {
	p, err := newPipelineBuild(deps, state)
	if err != nil {
		return err
	}
	p.gap = gap
	defer p.close()
	go p.decisions.run()
	engine, err := p.newEngine()
	if err != nil {
		return err
	}
	if deps.healthServer != nil {
		deps.healthServer.SetStatusSource(engine.Status)
		defer deps.healthServer.SetStatusSource(nil)
	}
	runners, err := p.newRunners(engine)
	if err != nil {
		return err
	}
	defer startSources(ctx, runners)()
	err = engine.Run(ctx)
	if !engine.WriterStopped() {
		guard.abandon()
	}
	return err
}

// newPipelineSourceHealth reports source diagnostics to /health when the
// health server exists.
func newPipelineSourceHealth(
	deps *serverDeps, source *kube.Source, dynamic *kube.DynamicSource,
) *sourceHealth {
	var sink sourceStatusSink
	if deps.healthServer != nil {
		sink = deps.healthServer
	}
	health := newSourceHealth(sink, source).withReadiness(
		func(ready bool) {
			deps.readiness.setCurrent("pipeline", ready)
		}).withHandlerPanics(kubeclient.HandlerPanics)
	if deps.healthServer != nil {
		health.withCoverage(coveragePublisher{
			sink: deps.healthServer, typed: source, dynamic: dynamic,
		})
	}
	return health
}

// startSources runs every source under a child context and returns a stop
// function that cancels them and waits for all to return, so none outlives
// the engine.
func startSources(
	ctx context.Context, runners []func(context.Context),
) (stop func()) {
	sourceCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	for _, run := range runners {
		background.Add(1)
		go func() {
			defer background.Done()
			run(sourceCtx)
		}()
	}
	return func() {
		cancel()
		background.Wait()
	}
}

// newDetectorRegistry builds the production registry from the shared
// default detector set, so production, replay and scenarios all run the
// same detectors. Only the detectors the edition is entitled to are
// registered; a nil e is Community. The Pro findings Community
// withholds are counted in withheld (optional).
func newDetectorRegistry(
	synced func(inventory.Kind) bool, e feature.Entitlements,
	withheld *feature.Withheld,
) *detection.Registry {
	return detection.NewRegistry(synced,
		detectors.Entitled(e, withheld)...)
}

// pruneModel drops old change history every modelPruneInterval. Expired
// baselines are removed by the pipeline's history writer, the only
// goroutine that writes baselines, so a delete never races a save.
func pruneModel(
	ctx context.Context, model *inventory.Model, now func() time.Time,
) {
	ticker := time.NewTicker(modelPruneInterval)
	defer ticker.Stop()
	var health modelHealthLog
	// A panic loses that pass only; the next tick prunes again.
	pass := func(prune bool) {
		safego.Do("model-prune", func() {
			if prune {
				pruneOnce(model, now())
			}
			health.log(model, now())
		})
	}
	pass(false)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pass(true)
		}
	}
}

// pruneOnce drops notes older than modelRetention and changes and health
// marks older than inventory.ChangeRetention, the day a restart reloads.
// It leaves baselines alone.
func pruneOnce(model *inventory.Model, now time.Time) {
	model.PruneKeeping(now.Add(-modelRetention),
		now.Add(-inventory.ChangeRetention))
}

// pipelineClock adapts the application clock to the pipeline's timer needs.
type pipelineClock struct {
	now interface{ Now() time.Time }
}

func (c pipelineClock) Now() time.Time { return c.now.Now() }

// timerClock is a clock that can also create timers, such as
// clock.RealClock or a test clock.
type timerClock interface {
	After(time.Duration) <-chan time.Time
}

// After uses the injected clock's timers when it has them, so a test
// clock controls pipeline waits too; otherwise it uses real time.
func (c pipelineClock) After(d time.Duration) <-chan time.Time {
	if timers, ok := c.now.(timerClock); ok {
		return timers.After(d)
	}
	return time.After(d)
}

// newActiveProber builds user-configured probes from the probe monitor
// configuration, when enabled.
func newActiveProber(
	deps *serverDeps, model inventory.Reader, submit kube.Submit,
) (*kube.ActiveProber, bool) {
	cfg := deps.runtime.ActiveProbe()
	if !cfg.Enabled || !deps.has(feature.ActiveProbes) {
		return nil, false
	}
	targets := probeTargets(cfg)
	excluded := map[string]bool{}
	for _, ns := range cfg.ExcludeNamespaces {
		excluded[ns] = true
	}
	return kube.NewActiveProber(kube.ActiveProbeConfig{
		Targets: targets, AutoServices: cfg.AutoServices, Excluded: excluded,
		AutoDependencies: cfg.AutoDependencies,
		Interval:         time.Duration(cfg.IntervalSeconds) * time.Second,
		Timeout:          time.Duration(cfg.TimeoutSeconds) * time.Second,
		FailureThreshold: cfg.FailureThreshold,
		// Probes use their own client: the provider proxy and CA bundle
		// are for alert traffic, not in-cluster endpoints.
		HTTPClient: deps.clients.ProbeHTTP,
		Resolver:   deps.clients.Resolver,
		Model:      model,
		Now:        deps.clients.Clock.Now,
		Submit:     submit,
	}), true
}

// probeTargets lists the configured probes. A target the KwatchConfig
// added is restricted: it must not reach loopback, link-local or
// metadata addresses, though in-cluster addresses are fine.
func probeTargets(cfg config.ActiveProbeMonitor) []kube.ProbeTarget {
	var targets []kube.ProbeTarget
	for _, t := range cfg.HTTP {
		targets = append(targets, kube.ProbeTarget{
			Name: t.Name, URL: t.URL, ExpectedStatus: t.ExpectedStatus,
			LatencyWarningMs:  t.LatencyWarningMs,
			LatencyCriticalMs: t.LatencyCriticalMs,
			Restricted:        t.FromOverlay,
		})
	}
	for _, t := range cfg.TCP {
		targets = append(targets, kube.ProbeTarget{
			Name: t.Name, Address: t.Address, Restricted: t.FromOverlay})
	}
	for _, t := range cfg.DNS {
		targets = append(targets, kube.ProbeTarget{Name: t.Name, Host: t.Host})
	}
	return targets
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
