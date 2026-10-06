package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/pipeline/investigate"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
	"github.com/abahmed/kwatch/internal/scope"
	"github.com/abahmed/kwatch/internal/storage"
)

// pipelineBuild holds the pieces runPipeline wires together, so each
// wiring step can be a small method.
type pipelineBuild struct {
	deps      *serverDeps
	state     *storage.Store
	clock     clock.Clock
	model     *inventory.Model
	scope     *scope.Scope
	auditLog  *audit.Logger
	incidents *incident.Manager
	decisions *decisionLog
	// source is set once the typed source exists. The detectors and the
	// incident manager read it through closures, so it may be nil while
	// they are built.
	source *kube.Source
}

// newPipelineBuild builds the model, scope, audit log, incident manager
// and decision log. The caller must call close.
func newPipelineBuild(
	deps *serverDeps, state *storage.Store,
) (*pipelineBuild, error) {
	p := &pipelineBuild{deps: deps, state: state, clock: deps.clients.Clock}
	p.model = inventory.NewModel(inventory.Options{
		EnrichmentSources: kube.EnrichmentSources(),
	})
	findingScope, err := scope.New(
		deps.runtime.Scope(), deps.runtime.Scope().Silences(),
		deps.runtime.Policy().Maintenance().Enabled, p.clock.Now)
	if err != nil {
		return nil, err
	}
	p.scope = findingScope
	auditCfg := deps.runtime.Lifecycle().AuditLog()
	p.auditLog = audit.NewLogger(audit.Config{
		Enabled: auditCfg.Enabled, Output: auditCfg.Output,
	})
	policy := deps.runtime.Policy()
	p.incidents = incident.NewManager(incident.Config{
		SeverityByReason:    policy.SeverityByReason(),
		SeverityByOwnerKind: policy.SeverityByOwnerKind(),
		Verifiable: func(kind inventory.Kind) bool {
			return p.source == nil || p.source.Verifiable(kind)
		},
	}, explain.NewSolver())
	// The pipeline marks a page open when it hands the message over;
	// delivery says when no pager took it after all.
	deps.deliveryManager.AttachPageObserver(func(key string) {
		p.incidents.RecordPaged(key, false)
	})
	p.decisions = newDecisionLog(
		p.auditLog.Record, metrics.DefaultRegistry(), p.incidents.Incidents)
	return p, nil
}

// close stops the decision log, then the audit log it writes to.
func (p *pipelineBuild) close() {
	p.decisions.closeWithin(decisionLogShutdown, time.After)
	_ = p.auditLog.Close()
}

func (p *pipelineBuild) synced(kind inventory.Kind) bool {
	return p.source != nil && p.source.Synced(kind)
}

// newEngine builds the decision engine. Its sink runs on the decision
// loop, so every step hands off without waiting: delivery queues the
// message and the audit entry goes to the decision log's worker.
func (p *pipelineBuild) newEngine() (*pipeline.Engine, error) {
	deps := p.deps
	return pipeline.NewEngine(pipeline.Dependencies{
		Model:     p.model,
		Detectors: newDetectorRegistry(p.synced),
		Incidents: p.incidents,
		Synced:    p.synced,
		Writer: compose.NewWriter(
			deps.runtime.Application().ClusterName,
			deps.runtime.Policy().Runbooks()),
		Sink:  p.recordDecision,
		Clock: pipelineClock{deps.clients.Clock},
		Store: pipeline.NewIncidentStore(p.state),
		Investigator: investigate.NewInvestigator(investigate.Sources{
			Model: p.model,
			Logs:  kube.LogReader{Client: deps.clients.Kubernetes}.Lines,
			Endpoints: kube.EndpointReader{
				Client: deps.clients.Kubernetes,
			}.ReadyEndpoints,
		}),
		InScope: pipeline.IncidentScope(p.model, p.scope),
		Progress: func() {
			deps.pipelineProgress.Touch(p.clock.Now())
		},
	})
}

func (p *pipelineBuild) recordDecision(
	_ context.Context, d incident.Decision, m notification.Message,
) {
	klog.V(1).InfoS("pipeline decision", "component", "pipeline",
		"incident", d.Incident.ID, "reason", d.Reason)
	counted := countDecision(metrics.DefaultRegistry(), d)
	p.decisions.record(pipeline.AuditEntry(d, m, p.clock.Now()), counted)
	p.deps.deliveryManager.NotifyIncident(m)
}

// newRunners builds every source and returns the functions that run them.
func (p *pipelineBuild) newRunners(
	engine *pipeline.Engine,
) ([]func(context.Context), error) {
	deps := p.deps
	maintenance := deps.runtime.Policy().Maintenance()
	// The dynamic source is built first: the typed source asks it whether
	// the kinds it does not watch itself are synced and verifiable.
	dynamicSource := kube.NewDynamicSource(
		dynamicSourceConfig(deps, engine.Submit, maintenance, p.model))
	source, err := kube.NewSource(typedSourceConfig(deps, engine.Submit,
		maintenance, dynamicSource, sourceDigestKey(p.state)))
	if err != nil {
		return nil, err
	}
	p.source = source
	stats := kube.NewStatsPoller(kube.StatsConfig{
		Kubelet:    kubeletReader(deps),
		Now:        p.clock.Now,
		Submit:     engine.Submit,
		Nodes:      entitiesOf(p.model, kube.KindNode),
		Containers: entitiesOf(p.model, kube.KindContainer),
		Report: newKubeletStatsHealth(
			healthSink(deps), metrics.DefaultRegistry()).report,
	})
	prober := kube.NewProber(kube.ProbeConfig{
		Client:   deps.clients.Kubernetes,
		Resolver: deps.clients.Resolver,
		Now:      p.clock.Now,
		Submit:   engine.Submit,
		HTTP:     deps.clients.ProbeHTTP,
		Model:    p.model,
	})
	crashLogs := kube.NewCrashLogRound(kube.CrashLogConfig{
		Logs:   kube.LogReader{Client: deps.clients.Kubernetes},
		Model:  p.model,
		Now:    p.clock.Now,
		Submit: engine.Submit,
	})
	runners := []func(context.Context){
		source.Run, stats.Run, dynamicSource.Run, prober.Run,
		crashLogs.Run,
		func(ctx context.Context) {
			pruneModel(ctx, p.model, p.clock.Now)
		},
		p.syncReporter(engine, newPipelineSourceHealth(
			deps, source, dynamicSource)),
	}
	probeModel := scopedServices{Reader: p.model, scope: p.scope}
	if active, ok := newActiveProber(deps, probeModel, engine); ok {
		runners = append(runners, active.Run)
	}
	return runners, nil
}

// syncReporter returns the runner that waits for the caches to sync, then
// marks the pipeline ready and reports source health until ctx ends.
func (p *pipelineBuild) syncReporter(
	engine *pipeline.Engine, health *sourceHealth,
) func(context.Context) {
	return func(ctx context.Context) {
		if !p.source.WaitForSync(ctx) {
			return
		}
		// The first report sets pipeline readiness from the required
		// kinds; later reports withdraw or restore it.
		health.report()
		engine.SourcesSynced()
		ticker := time.NewTicker(sourceHealthInterval)
		defer ticker.Stop()
		health.run(ctx, ticker.C)
	}
}

// entitiesOf lists the entities of a kind the model has now.
func entitiesOf(
	model inventory.Reader, kind inventory.Kind,
) func() []inventory.EntityID {
	return func() []inventory.EntityID { return model.Entities(kind) }
}
