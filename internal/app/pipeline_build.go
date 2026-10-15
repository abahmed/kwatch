package app

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/flight"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/kubeclient"
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
	// flight records the observations the sources submit (flight.go).
	flight *flight.Recorder
}

// newPipelineBuild builds the model, scope, audit log, incident manager
// and decision log. The caller must call close.
func newPipelineBuild(
	deps *serverDeps, state *storage.Store,
) (*pipelineBuild, error) {
	p := &pipelineBuild{deps: deps, state: state, clock: deps.clients.Clock}
	if deps.has(FlightRecorder) {
		p.flight = newFlightRecorder(state, p.clock.Now)
	}
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
		Enabled: auditCfg.Enabled && deps.has(feature.AuditLog),
		Output:  auditCfg.Output,
	})
	p.incidents = incident.NewManager(p.incidentConfig(),
		explain.NewSolver())
	// The pipeline marks a page open when it hands the message over;
	// delivery says when no pager took it after all.
	deps.deliveryManager.AttachPageObserver(func(key string) {
		p.incidents.RecordPaged(key, false)
	})
	p.decisions = newDecisionLog(
		p.auditLog.Record, metrics.DefaultRegistry(), p.incidents.Incidents)
	return p, nil
}

// incidentConfig is the lifecycle configuration of the edition. Without
// the Pro lifecycle there are no flapping incidents, paging rules,
// reminders or hand-overs, and no severity overrides.
func (p *pipelineBuild) incidentConfig() incident.Config {
	deps := p.deps
	cfg := incident.Config{
		Verifiable: func(kind inventory.Kind) bool {
			return p.source == nil || p.source.Verifiable(kind)
		},
	}
	if deps.has(feature.SeverityOverrides) {
		policy := deps.runtime.Policy()
		cfg.SeverityByReason = policy.SeverityByReason()
		cfg.SeverityByOwnerKind = policy.SeverityByOwnerKind()
	}
	if !deps.has(feature.Flapping) {
		cfg.NoFlapping, cfg.NoPaging = true, true
		cfg.NoReminders, cfg.NoHandover = true, true
	}
	return cfg
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
		Model:        p.model,
		Detectors:    newDetectorRegistry(p.synced, deps.entitlements),
		Incidents:    p.incidents,
		Synced:       p.synced,
		Writer:       p.newWriter(),
		Sink:         p.recordDecision,
		Clock:        pipelineClock{deps.clients.Clock},
		Store:        pipeline.NewIncidentStore(p.state),
		Investigator: p.newInvestigator(),
		InScope:      pipeline.IncidentScope(p.model, p.scope),
		Progress: func() {
			deps.pipelineProgress.Touch(p.clock.Now())
		},
		Entitlements: deps.entitlements,
		InactivePro:  deps.inactivePro,

		ProDetectorsOff: deps.notice.detectors,
		CommunitySends:  deps.notice.sends,
	})
}

// newWriter composes messages for the edition: Community writes the
// one-line cause without proofs, and runbooks need Pro.
func (p *pipelineBuild) newWriter() compose.Writer {
	app := p.deps.runtime.Application()
	var runbooks map[string]string
	if p.deps.has(feature.Runbooks) {
		runbooks = p.deps.runtime.Policy().Runbooks()
	}
	return compose.NewWriter(app.ClusterName, runbooks).
		ForEdition(p.deps.entitlements, app.HideProHints)
}

// newInvestigator reads logs and endpoints for the incidents it
// investigates. Without investigation nothing is read and no excerpt is
// collected.
func (p *pipelineBuild) newInvestigator() investigate.Investigator {
	if !p.deps.has(feature.Investigation) {
		return nil
	}
	client := p.deps.clients.Kubernetes
	return investigate.NewInvestigator(investigate.Sources{
		Model:       p.model,
		Logs:        kube.LogReader{Client: client}.Lines,
		CurrentLogs: kube.LogReader{Client: client}.CurrentLines,
		Endpoints:   kube.EndpointReader{Client: client}.ReadyEndpoints,
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

// newProber builds the prober of the API server, cluster DNS and the
// Leases of controllers.
func (p *pipelineBuild) newProber(submit kube.Submit) *kube.Prober {
	deps := p.deps
	return kube.NewProber(kube.ProbeConfig{
		Client:   deps.clients.Kubernetes,
		Resolver: deps.clients.Resolver,
		Now:      p.clock.Now,
		Submit:   submit,
		HTTP:     deps.clients.ProbeHTTP,
		Model:    p.model,
		// kwatch renews its own leader Lease; it is not a controller.
		OwnLeaseNamespace: kubeclient.GetNamespace(),
		OwnLeaseName:      electionLeaseName(),
	})
}

// newRunners builds every source and returns the functions that run them.
func (p *pipelineBuild) newRunners(
	engine *pipeline.Engine,
) ([]func(context.Context), error) {
	deps := p.deps
	submit := recordedSubmit(p.flight, engine.Submit)
	maintenance := deps.runtime.Policy().Maintenance()
	// The dynamic source is built first: the typed source asks it whether
	// the kinds it does not watch itself are synced and verifiable.
	dynamicSource := kube.NewDynamicSource(
		dynamicSourceConfig(deps, submit, maintenance, p.model))
	source, err := kube.NewSource(typedSourceConfig(deps, submit,
		maintenance, dynamicSource, sourceDigestKey(p.state)))
	if err != nil {
		return nil, err
	}
	p.source = source
	stats := kube.NewStatsPoller(kube.StatsConfig{
		Kubelet:    kubeletReader(deps),
		Now:        p.clock.Now,
		Submit:     submit,
		Nodes:      entitiesOf(p.model, kube.KindNode),
		Containers: entitiesOf(p.model, kube.KindContainer),
		Report: newKubeletStatsHealth(
			healthSink(deps), metrics.DefaultRegistry()).report,
	})
	crashLogs := kube.NewCrashLogRound(kube.CrashLogConfig{
		Logs:   kube.LogReader{Client: deps.clients.Kubernetes},
		Model:  p.model,
		Now:    p.clock.Now,
		Submit: submit,
	})
	runners := []func(context.Context){
		source.Run, dynamicSource.Run, crashLogs.Run,
		func(ctx context.Context) {
			pruneModel(ctx, p.model, p.clock.Now)
		},
		p.syncReporter(engine, newPipelineSourceHealth(
			deps, source, dynamicSource)),
	}
	// The kubelet stats and the control plane probes feed detectors
	// that only Pro runs; Community does not read them.
	if deps.has(feature.NodeUsage) {
		runners = append(runners, stats.Run)
	}
	if deps.has(feature.ControlPlane) {
		runners = append(runners, p.newProber(submit).Run)
	}
	if p.flight != nil {
		runners = append(runners, p.flight.Run)
	}
	probeModel := scopedServices{Reader: p.model, scope: p.scope}
	if active, ok := newActiveProber(deps, probeModel, submit); ok {
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
		// The startup hold ends with the required kinds: a slow or
		// absent optional kind is carried (see Engine.reconcileDowntime)
		// and must not delay the first alerts of a cold start.
		if !p.source.WaitForRequiredSync(ctx) {
			return
		}
		engine.SourcesSynced()
		if !p.source.WaitForSync(ctx) {
			return
		}
		// The first report sets pipeline readiness from the required
		// kinds; later reports withdraw or restore it.
		health.report()
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
