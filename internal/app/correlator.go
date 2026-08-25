package app

import (
	"context"
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
