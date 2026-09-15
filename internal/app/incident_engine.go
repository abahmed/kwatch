package app

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/delivery"
	"github.com/abahmed/kwatch/internal/enricher"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// configureDelivery starts delivery after InitRuntime has compiled the
// immutable routing, template, and silence snapshot.
func configureDelivery(
	ctx context.Context,
	am *delivery.Manager,
) {
	am.Start(ctx)
}

// engineHolder carries the engine through hook registration so the hooks can
// reference it after construction completes.
type engineHolder struct {
	engine *incident.Engine
}

// newIncidentEngine builds the incident engine with its lifecycle,
// mass-failure, and baseline hooks wired to the alerting stack.
func newIncidentEngine(
	runtime config.RuntimeConfig,
	now func() time.Time,
	subjectPresent func(resource, namespace, name string) (bool, bool),
	baseline map[string]map[string]int64,
	am *delivery.Manager,
	auditLogger *audit.AuditLogger,
	graph *kwcontext.ResourceGraph,
	baselineCh chan map[string]map[string]int64,
	insightEngine *insight.Engine,
	feedbackStore *insight.FeedbackStore,
	saveFeedback func(),
) *incident.Engine {
	incidentConfig := runtime.Incident()
	holder := &engineHolder{}
	opts := &engineOptions{
		baseline:        baseline,
		deliveryManager: am,
		auditLogger:     auditLogger,
		graph:           graph,
		baselineCh:      baselineCh,
		insightEngine:   insightEngine,
		feedbackStore:   feedbackStore,
		saveFeedback:    saveFeedback,
		notify:          am.NotifyIncident,
	}

	holder.engine = incident.NewEngineWithClock(incident.Config{
		Window:            incidentConfig.Window,
		LifecycleInterval: incidentConfig.LifecycleInterval,
		Baseline:          baseline,
		Enricher: &enricher.DefaultEnricher{
			SeverityByOwnerKind: runtime.SeverityByOwnerKind(),
			SeverityByReason:    runtime.SeverityByReason(),
		},
		AuditLogger:                auditLogger,
		EscalationEnabled:          incidentConfig.EscalationEnabled,
		EscalationTiers:            incidentConfig.EscalationTiers,
		InhibitNodeSuppressesPods:  incidentConfig.InhibitNodeSuppressesPods,
		RenotifyIntervalBySeverity: incidentConfig.RenotifyIntervalBySeverity,
		RenotifyMaxPerIncident:     incidentConfig.RenotifyMaxPerIncident,
		Runbooks:                   runtime.Runbooks(),
		SubjectPresent:             subjectPresent,
		ResolveHoldDown:            incidentConfig.ResolveHoldDown,
		MaxBaseline:                incidentConfig.MaxBaseline,
		SmartGroupingWindow:        incidentConfig.SmartGroupingWindow,
		NamespaceFanOutThreshold:   incidentConfig.NamespaceFanOutThreshold,
		DependenciesOf:             dependencyResolver(opts.graph),
		LifecycleHook:              lifecycleHook(opts, holder),
		MassFailureHook:            massFailureHook(opts, holder),
		OnBaselineChange:           onBaselineChange(baselineCh),
	}, clock.Func(now))
	return holder.engine
}

func dependencyResolver(
	graph *kwcontext.ResourceGraph,
) func(*model.Incident) []string {
	return func(inc *model.Incident) []string {
		return insight.DependenciesFor(graph, inc)
	}
}

// engineOptions groups the inputs shared by the incidentEngine hooks.
type engineOptions struct {
	baseline        map[string]map[string]int64
	deliveryManager *delivery.Manager
	auditLogger     *audit.AuditLogger
	graph           *kwcontext.ResourceGraph
	baselineCh      chan<- map[string]map[string]int64
	// insightEngine turns an incident plus the resource graph into a
	// diagnosis: likely cause, impact, what changed recently.
	insightEngine *insight.Engine
	feedbackStore *insight.FeedbackStore
	saveFeedback  func()
	// notify is the delivery sink, injectable so the hook can be tested.
	notify func(*model.Incident, model.IncidentAction, *insight.Insight)
}
