package app

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/alert"
	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/config"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/state"
)

// configureAlertManager applies silences, templates, and starts delivery.
func configureAlertManager(
	ctx context.Context,
	cfg *config.Config,
	am *alert.AlertManager,
) {
	am.SetSilences(cfg.Silences)
	am.SetTemplates(cfg.Templates)
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
	cfg *config.Config,
	baseline map[string]map[string]int64,
	am *alert.AlertManager,
	auditLogger *audit.AuditLogger,
	graph *kwcontext.ResourceGraph,
	baselineCh chan map[string]map[string]int64,
	insightEngine *insight.Engine,
) *correlation.Engine {
	holder := &engineHolder{}
	opts := &engineOptions{
		cfg:           cfg,
		baseline:      baseline,
		alertManager:  am,
		auditLogger:   auditLogger,
		graph:         graph,
		baselineCh:    baselineCh,
		insightEngine: insightEngine,
		notify:        am.NotifyIncident,
	}

	holder.engine = correlation.NewEngine(correlation.Config{
		Window: time.Duration(
			cfg.Correlation.Window,
		) * time.Minute,
		LifecycleInterval: time.Duration(
			cfg.Correlation.LifecycleInterval,
		) * time.Minute,
		Baseline: baseline,
		Enricher: &enricher.DefaultEnricher{
			SeverityByOwnerKind: cfg.SeverityByOwnerKind,
			SeverityByReason:    cfg.SeverityByReason,
		},
		EscalationEnabled:         cfg.Correlation.Escalation.Enabled,
		EscalationTiers:           cfg.Correlation.Escalation.Tiers,
		InhibitNodeSuppressesPods: cfg.Inhibition.NodeSuppressesPods,
		RenotifyIntervalBySeverity: renotifyIntervalBySeverity(
			cfg.Correlation.Renotify.IntervalBySeverity,
		),
		RenotifyMaxPerIncident: cfg.Correlation.Renotify.MaxPerIncident,
		Runbooks:               cfg.Runbooks,
		ResolveHoldDown: time.Duration(
			cfg.Correlation.ResolveHoldDown,
		) * time.Second,
		MaxBaseline: cfg.Correlation.MaxBaseline,
		SmartGroupingWindow: time.Duration(
			cfg.SmartGrouping.WindowSeconds,
		) * time.Second,
		NamespaceFanOutThreshold: cfg.SmartGrouping.NamespaceFanOutThreshold,
		DependenciesOf: func(inc *model.Incident) []string {
			return insight.DependenciesFor(opts.graph, inc)
		},
		LifecycleHook:    lifecycleHook(opts, holder),
		MassFailureHook:  massFailureHook(opts, holder),
		OnBaselineChange: onBaselineChange(baselineCh),
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
	// insightEngine turns an incident plus the resource graph into a
	// diagnosis: likely cause, impact, what changed recently.
	insightEngine *insight.Engine
	// notify is the delivery sink, injectable so the hook can be tested.
	notify func(*model.Incident, model.IncidentAction, *insight.Insight)
}

// diagnose runs the insight engine for the actions where a diagnosis helps.
// A resolve carries none: there is nothing left to explain. A mass failure is
// the diagnosis — its hint already names the shared dependency — so running
// the graph over it again would only add noise.
func (o *engineOptions) diagnose(
	inc *model.Incident,
	action model.IncidentAction,
) *insight.Insight {
	if o.insightEngine == nil || action == model.ActionResolved ||
		correlation.IsMassFailureKey(inc.Key) {
		return nil
	}
	return o.insightEngine.Analyze(inc)
}

// lifecycleHook audits and notifies each incident edge unless it was skipped.
// It is the only place a notification is sent from: the engine routes every
// decision — live events, resolves, group flushes, renotify, mass failures —
// through here, so audit, diagnosis and delivery cannot diverge between paths.
func lifecycleHook(
	opts *engineOptions,
	holder *engineHolder,
) func(*model.Incident, model.IncidentAction) {
	return func(inc *model.Incident, action model.IncidentAction) {
		if action != model.ActionSkip {
			opts.auditLogger.LogIncident(inc, action)
			opts.notify(inc, action, opts.diagnose(inc, action))
		}
		metrics.Default.ActiveIncidents.Store(
			int64(holder.engine.ActiveCount()),
		)
	}
}

// massFailureHook reports new mass failures and resolves ones that cleared.
func massFailureHook(opts *engineOptions, holder *engineHolder) func() {
	return func() {
		allIncidents := holder.engine.ActiveIncidents()
		incList := make([]*model.Incident, 0, len(allIncidents))
		for _, inc := range allIncidents {
			incList = append(incList, inc)
		}
		mfs := insight.ScanMassFailures(incList, opts.graph)

		current := make(map[string]insight.MassFailure, len(mfs))
		for _, mf := range mfs {
			current[mf.SharedDependency] = mf
		}
		notifyNewMassFailures(holder, current)
		resolveClearedMassFailures(holder, current)
	}
}

// notifyNewMassFailures fires active incidents for mass failures the engine is
// not yet tracking.
func notifyNewMassFailures(
	holder *engineHolder,
	current map[string]insight.MassFailure,
) {
	for key, mf := range current {
		incKey := correlation.MassFailureKey(key)
		if holder.engine.HasMassFailure(incKey) {
			continue
		}
		klog.V(2).InfoS("mass failure detected", "message", mf.Describe())
		now := time.Now()
		inc := &model.Incident{
			Subject: model.Subject{
				ID:        massFailureID(incKey),
				Key:       incKey,
				Reason:    mf.Reason,
				Namespace: mf.Namespace,
				Resource:  mf.ResourceKind,
				Name:      describeDependency(key),
			},
			Status: model.Status{
				Count:         mf.AffectedCount,
				PeakResources: mf.AffectedCount,
				FirstSeen:     now,
				LastSeen:      now,
				State:         model.StateActive,
			},
			Evidence: model.Evidence{
				Hint: mf.Describe(),
			},
		}

		holder.engine.AddMassFailure(inc)
	}
}

// massFailureID derives the stable short id every provider displays. Group
// incidents use the same crc32-of-key scheme, so the two look consistent.
func massFailureID(key model.IncidentKey) string {
	return fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(key)))
}

// describeDependency turns the internal "kind/namespace/name" dependency key
// into something readable. Cluster-scoped resources carry an empty namespace,
// which rendered as "node//ip-10-0-0-1" before.
func describeDependency(depKey string) string {
	parts := strings.SplitN(depKey, "/", 3)
	if len(parts) != 3 {
		return depKey
	}
	kind, ns, name := parts[0], parts[1], parts[2]
	if ns == "" {
		return kind + " " + name
	}
	return kind + " " + ns + "/" + name
}

// resolveClearedMassFailures resolves tracked mass failures whose shared
// dependency no longer appears in the failure set.
func resolveClearedMassFailures(
	holder *engineHolder,
	current map[string]insight.MassFailure,
) {
	tracked := holder.engine.MassFailureSet()
	for incKey := range tracked {
		dep := strings.TrimPrefix(string(incKey), "mass-failure/")
		if _, exists := current[dep]; exists {
			continue
		}
		klog.V(2).InfoS("mass failure resolved", "dependency", dep)
		// The engine announces the resolve and then releases whatever the
		// mass failure was speaking for that is still broken, so a symptom
		// that outlives its cause is announced instead of lost.
		holder.engine.RemoveMassFailure(incKey)
	}
}

// onBaselineChange forwards baseline snapshots to the persistent saver.
func onBaselineChange(
	baselineCh chan map[string]map[string]int64,
) func(map[string]map[string]int64) {
	return func(b map[string]map[string]int64) {
		total := 0
		for _, pods := range b {
			total += len(pods)
		}
		metrics.Default.BaselineSize.Store(int64(total))
		select {
		case baselineCh <- b:
		default:
			// Channel full: drop oldest, keep newest.
			select {
			case <-baselineCh:
			default:
			}
			select {
			case baselineCh <- b:
			default:
			}
		}
	}
}

// restoreIncidents loads previously persisted incidents back into memory.
func restoreIncidents(
	ctx context.Context,
	stateMgr *state.StateManager,
	correlator *correlation.Engine,
	allowed func(string) bool,
) {
	persisted, err := stateMgr.LoadPersistedIncidents(ctx)
	if err != nil {
		// Not fatal: kwatch starts with no correlation memory, which means
		// anything already broken is announced again as new. Worth a log, not
		// worth refusing to start.
		klog.ErrorS(err, "failed to restore incidents from configmap")
		return
	}
	correlator.FilterBaseline(allowed)
	if len(persisted) == 0 {
		return
	}
	restored := make(map[model.IncidentKey]*model.Incident, len(persisted))
	for i := range persisted {
		inc := persisted[i].ToIncident()
		if inc.Namespace != "" && allowed != nil && !allowed(inc.Namespace) {
			continue
		}
		restored[inc.Key] = inc
	}
	correlator.RestoreIncidents(restored)
	klog.InfoS("restored incidents from configmap", "count", len(persisted))
}
