package insight

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

const (
	maxAnalysisStates  = 2048
	maxTransitions     = 12
	maxClassifierBytes = 16 * 1024
	flapWindow         = 30 * time.Minute
	rolloutGrace       = 10 * time.Minute
)

type analysisState struct {
	lastSeen    time.Time
	lastAction  model.IncidentAction
	root        model.ObjectRef
	transitions []time.Time
	diagnosis   string
	suppressed  time.Time
	delivered   bool
}

func (e *Engine) applyIntelligence(inc *model.Incident, ins *Insight) {
	if inc == nil || ins == nil {
		return
	}
	e.applyReplicaComparison(inc, ins)
	e.applyAvailabilitySeverity(inc, ins)
	e.applyTimeline(inc, ins)
	e.appendPropagationTimeline(ins)
	e.applyMaintenanceContext(ins)
	e.applyBaseline(inc, ins)
	e.applyHistory(inc, ins)
	e.applyUnknownSummary(inc, ins)
	e.applyRolloutSuppression(inc, ins)
	e.applyLogSignal(inc, ins)
	recordInsightQuality(ins)
}

func (e *Engine) appendPropagationTimeline(ins *Insight) {
	if ins.RootCause.Name == "" {
		return
	}
	ins.Timeline = append(ins.Timeline, TimelineEntry{
		At: e.now(), Kind: "analysis",
		Summary: fmt.Sprintf(
			"%s was identified as the leading upstream cause",
			ins.RootCause.Describe(),
		),
	})
}

func (e *Engine) applyReplicaComparison(
	inc *model.Incident,
	ins *Insight,
) {
	desired := inc.Facts.DesiredReplicas
	ready := inc.Facts.ReadyReplicas
	if desired <= 0 {
		return
	}
	failing := desired - ready
	if failing < 0 {
		failing = 0
	}
	ins.ReplicaState = fmt.Sprintf(
		"%d/%d replicas are ready; %d are failing", ready, desired, failing,
	)
	if ready > 0 && failing > 0 {
		ins.Contradictions = appendUniqueStrings(
			ins.Contradictions,
			"healthy replicas show the failure is not workload-wide",
		)
	}
}

func (e *Engine) applyAvailabilitySeverity(
	inc *model.Incident,
	ins *Insight,
) {
	ins.Severity = inc.Severity
	completeOutage := inc.Facts.EndpointsObserved &&
		inc.Facts.HealthyEndpoints == 0
	if inc.Facts.DesiredReplicas > 0 && inc.Facts.ReadyReplicas == 0 {
		completeOutage = true
	}
	if completeOutage && ins.Severity.Rank() < model.SeverityCritical.Rank() {
		ins.Severity = model.SeverityCritical
		return
	}
	partial := inc.Facts.DesiredReplicas > inc.Facts.ReadyReplicas &&
		inc.Facts.ReadyReplicas > 0
	if partial && ins.Severity.Rank() < model.SeverityWarning.Rank() {
		ins.Severity = model.SeverityWarning
	}
}

func (e *Engine) applyTimeline(inc *model.Incident, ins *Insight) {
	if !inc.FirstSeen.IsZero() {
		ins.Timeline = append(ins.Timeline, TimelineEntry{
			At: inc.FirstSeen, Kind: "failure",
			Summary: "the first related failure was observed",
		})
	}
	for _, change := range ins.RecentChanges {
		ins.Timeline = append(ins.Timeline, TimelineEntry{
			At: change.Timestamp, Kind: "change",
			Summary: fmt.Sprintf(
				"%s %s changed", change.Resource, change.Name,
			),
		})
	}
	sort.SliceStable(ins.Timeline, func(i, j int) bool {
		return ins.Timeline[i].At.Before(ins.Timeline[j].At)
	})
	if len(ins.Timeline) > 6 {
		ins.Timeline = ins.Timeline[len(ins.Timeline)-6:]
	}
}

func (e *Engine) applyMaintenanceContext(ins *Insight) {
	if len(ins.RecentChanges) == 0 {
		return
	}
	change := ins.RecentChanges[0]
	if !workloadKinds[strings.ToLower(change.Resource)] {
		return
	}
	ins.Maintenance = fmt.Sprintf(
		"a %s rollout started %s before the failure",
		change.Resource, ageOf(change.Timestamp, e.now()),
	)
	if ins.CauseState == CauseUnknown {
		ins.Provisional = true
	}
}

func (e *Engine) applyBaseline(inc *model.Incident, ins *Insight) {
	if e.feedback == nil {
		return
	}
	prefix := strings.ToLower(strings.TrimSpace(inc.Reason)) + "|"
	for _, record := range e.feedback.Snapshot() {
		if !strings.HasPrefix(record.Fingerprint, prefix) ||
			record.Observations < 3 {
			continue
		}
		ins.Baseline = fmt.Sprintf(
			"this failure pattern has been observed %d times and recurred %d "+
				"times", record.Observations, record.Recurred,
		)
		break
	}
}

func (e *Engine) applyRolloutSuppression(
	inc *model.Incident,
	ins *Insight,
) {
	rolloutCause := ins.Pattern == "rollout" ||
		ins.Pattern == "rollout_failure"
	if ins.Maintenance == "" ||
		(ins.CauseState != CauseUnknown && !rolloutCause) {
		e.clearRolloutSuppression(inc.Key)
		return
	}
	available := inc.Facts.ReadyReplicas > 0 ||
		(inc.Facts.EndpointsObserved && inc.Facts.HealthyEndpoints > 0)
	if !available {
		e.clearRolloutSuppression(inc.Key)
		return
	}
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	state := e.states[inc.Key]
	if state.suppressed.IsZero() {
		state.suppressed = e.now()
	}
	if e.now().Sub(state.suppressed) < rolloutGrace {
		ins.SuppressReason = "expected_rollout_with_capacity"
	} else {
		ins.Evidence = appendUniqueStrings(
			ins.Evidence,
			"the rollout remained degraded beyond the 10m grace period",
		)
		ins.Maintenance = "the rollout is still degraded after the 10m " +
			"grace period"
	}
	e.states[inc.Key] = state
}

func (e *Engine) clearRolloutSuppression(key model.IncidentKey) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	state, ok := e.states[key]
	if !ok || state.suppressed.IsZero() {
		return
	}
	state.suppressed = time.Time{}
	e.states[key] = state
}

// ShouldAnnounceReevaluation reports whether graph-driven analysis changed
// enough to justify an update. Normal lifecycle updates bypass this filter.
func (e *Engine) ShouldAnnounceReevaluation(
	inc *model.Incident,
	ins *Insight,
) bool {
	if inc == nil || ins == nil {
		return false
	}
	signature := diagnosisSignature(ins)
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	state := e.states[inc.Key]
	changed := state.diagnosis != signature
	state.diagnosis = signature
	e.states[inc.Key] = state
	return changed
}

func diagnosisSignature(ins *Insight) string {
	return fmt.Sprintf(
		"%s|%s|%s|%s|%s|%s|%t",
		ins.CauseState,
		ins.RootCause.Key(),
		ins.Pattern,
		ins.Impact,
		ins.Severity,
		ins.SuppressReason,
		ins.Flapping != nil,
	)
}

// DeliveryAction preserves user-visible lifecycle truth when an incident's
// initial notification was suppressed. Its first eventual delivery is a
// create, even if the incident engine has observed internal updates.
func (e *Engine) DeliveryAction(
	inc *model.Incident,
	requested model.IncidentAction,
) model.IncidentAction {
	if inc == nil || requested == model.ActionResolved {
		return requested
	}
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	state := e.states[inc.Key]
	if !state.delivered {
		return model.ActionCreate
	}
	return requested
}

func (e *Engine) RecordDelivery(inc *model.Incident) {
	if inc == nil || inc.State == model.StateResolved {
		return
	}
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	state := e.states[inc.Key]
	state.delivered = true
	e.states[inc.Key] = state
}

func (e *Engine) applyHistory(inc *model.Incident, ins *Insight) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	state, ok := e.states[inc.Key]
	if ok {
		ins.Reevaluated = true
		if state.root.Name != "" && state.root.Key() != ins.RootCause.Key() {
			ins.Evidence = appendUniqueStrings(
				ins.Evidence,
				"the leading cause changed as new cluster evidence arrived",
			)
		}
	}
	state.lastSeen = e.now()
	state.root = ins.RootCause
	e.states[inc.Key] = state
	if len(state.transitions) >= 3 {
		ins.Flapping = &FlappingSummary{
			Transitions: len(state.transitions), Window: flapWindow,
		}
	}
	e.pruneStatesLocked()
}

func (e *Engine) observeState(
	inc *model.Incident,
	action model.IncidentAction,
) {
	if inc == nil {
		return
	}
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	state := e.states[inc.Key]
	now := e.now()
	if state.lastSeen.IsZero() || state.lastAction != action {
		state.transitions = append(state.transitions, now)
	}
	cutoff := now.Add(-flapWindow)
	kept := state.transitions[:0]
	for _, transition := range state.transitions {
		if !transition.Before(cutoff) {
			kept = append(kept, transition)
		}
	}
	if len(kept) > maxTransitions {
		kept = kept[len(kept)-maxTransitions:]
	}
	state.transitions = kept
	state.lastAction = action
	state.lastSeen = now
	if action == model.ActionResolved {
		state.suppressed = time.Time{}
		state.diagnosis = ""
		state.delivered = false
	}
	e.states[inc.Key] = state
	e.pruneStatesLocked()
}

func (e *Engine) pruneStatesLocked() {
	if len(e.states) <= maxAnalysisStates {
		return
	}
	var oldestKey model.IncidentKey
	var oldest time.Time
	for key, state := range e.states {
		if oldest.IsZero() || state.lastSeen.Before(oldest) {
			oldestKey, oldest = key, state.lastSeen
		}
	}
	delete(e.states, oldestKey)
}

func (e *Engine) applyUnknownSummary(
	inc *model.Incident,
	ins *Insight,
) {
	if ins.CauseState != CauseUnknown {
		return
	}
	checked := len(ins.Candidates)
	ins.UnknownSummary = fmt.Sprintf(
		"Kwatch checked %d cause candidates and found no confirmed shared "+
			"failure for %s; the alert includes the strongest observed signals",
		checked, inc.Ref().Describe(),
	)
}

func (e *Engine) applyLogSignal(inc *model.Incident, ins *Insight) {
	if e.logClassifier == nil || !inc.IncludeLogs || inc.Logs == "" {
		return
	}
	logs := inc.Logs
	if len(logs) > maxClassifierBytes {
		logs = logs[len(logs)-maxClassifierBytes:]
	}
	ins.LogSignal = e.logClassifier.Classify(logs)
}

func recordInsightQuality(ins *Insight) {
	registry := metrics.DefaultRegistry()
	registry.InsightAnalyses.Add(1)
	switch ins.CauseState {
	case CauseConfirmed:
		registry.InsightConfirmed.Add(1)
	case CauseLikely:
		registry.InsightLikely.Add(1)
	default:
		registry.InsightUnknown.Add(1)
	}
	if ins.Reevaluated {
		registry.InsightReevaluations.Add(1)
	}
}
