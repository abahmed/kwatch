package app

import (
	"context"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/pipeline"
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
	guard *storeGuard,
) error {
	p, err := newPipelineBuild(deps, state)
	if err != nil {
		return err
	}
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
		})
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
// same detectors.
func newDetectorRegistry(synced func(inventory.Kind) bool) *detection.Registry {
	return detection.NewRegistry(synced, detectors.Default()...)
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
	health.log(model, now())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruneOnce(model, now())
			health.log(model, now())
		}
	}
}

// pruneOnce drops change history older than modelRetention. It leaves
// baselines alone.
func pruneOnce(model *inventory.Model, now time.Time) {
	model.Prune(now.Add(-modelRetention))
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
	deps *serverDeps, model inventory.Reader, engine *pipeline.Engine,
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
		Submit:     engine.Submit,
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
