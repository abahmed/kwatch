package detectors

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Grace periods of the generic health rules. Controllers pass through
// every one of these states briefly while they work.
const (
	// DefaultConditionGrace matches the custom resource detector so both
	// see a failing condition in the same evaluation and deduplicate.
	DefaultConditionGrace = DefaultCustomFailing
	// DefaultGenerationGrace is how long observedGeneration may trail
	// generation before the controller counts as not reconciling.
	DefaultGenerationGrace = 5 * time.Minute
	// DefaultDeletionGrace is how long finalizers may hold a deletion.
	DefaultDeletionGrace = 10 * time.Minute
	// DefaultPendingGrace is how long an object may stay in phase Pending.
	DefaultPendingGrace = 10 * time.Minute
)

// Generic derives health from the attributes every object carries
// (generic_attributes.go): status conditions, generation lag, blocked
// deletion and phase. It runs for every entity, without per-kind code.
//
// Deduplication rule: Generic is a detection.Fallback. Kinds with a
// detector registered for their own kind (pods, nodes, workloads,
// claims...) own their conditions, phase and deletion, so Generic only
// adds generation lag there. On every kind the registry drops a generic
// finding whose Mode a specialised finding of the same entity reports,
// and a generic condition finding whenever a specialised finding exists
// (for example the custom resource detector on a failing CR).
type Generic struct{}

// Name implements detection.Detector.
func (Generic) Name() string { return "generic-health" }

// Kinds implements detection.Detector.
func (Generic) Kinds() []inventory.Kind {
	return []inventory.Kind{detection.AnyKind}
}

// Fallback implements detection.Fallback.
func (Generic) Fallback() {}

// Detect implements detection.Detector.
func (Generic) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if len(e.Attributes) == 0 {
		return nil
	}
	var out []detection.Finding
	if f, ok := generationLag(ctx, e); ok {
		out = append(out, f)
	}
	if ctx.Specialised() {
		// Few specialised detectors look at deletion, so a claim or a
		// workload held by a finalizer is reported here. Pods, nodes and
		// namespaces judge their own deletion.
		if f, ok := stuckDeletion(ctx, e); ok && !ownsDeletion[e.ID.Kind] {
			out = append(out, f)
		}
		return out
	}
	out = append(out, badConditions(ctx, e)...)
	if f, ok := stuckDeletion(ctx, e); ok {
		out = append(out, f)
	}
	if f, ok := badPhase(ctx, e); ok {
		out = append(out, f)
	}
	return out
}

func badConditions(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	for _, rule := range kube.ConditionRules {
		status, reason, since := condition(e, rule.Type)
		key := "cond/" + rule.Type + "/" + rule.Bad
		if status != rule.Bad ||
			!sustained(ctx, key, since, conditionGrace(rule)) {
			continue
		}
		out = append(out, conditionFinding(e, rule, reason, since))
	}
	return out
}

// conditionGrace is how long a condition must hold. One that says the
// controller cannot tell yet (Unknown) gets the reconcile window, the
// same one custom resources use.
func conditionGrace(rule kube.ConditionRule) time.Duration {
	if rule.Bad == "Unknown" {
		return DefaultGenerationGrace
	}
	return DefaultConditionGrace
}

func conditionFinding(
	e inventory.Entity, rule kube.ConditionRule, reason string,
	since time.Time,
) detection.Finding {
	summary := rule.Type + " is " + rule.Bad
	if reason != "" {
		summary += " (" + reason + ")"
	}
	evidence := []detection.Evidence{{
		Label: "condition", Value: rule.Type + "=" + rule.Bad,
	}}
	if reason != "" {
		evidence = append(evidence,
			detection.Evidence{Label: "reason", Value: reason})
	}
	message := text(e, kube.ConditionKey(rule.Type)+
		kube.AttrConditionMessage)
	if message != "" {
		evidence = append(evidence,
			detection.Evidence{Label: "message", Value: message})
	}
	health := detection.Degraded
	if rule.Failing {
		health = detection.Failing
	}
	return detection.Finding{
		Reason: reasons.Condition(rule.Type), Severity: detection.Warning,
		Health: health, Mode: detection.ConditionModeOf(rule.Type),
		Since: since, Summary: summary, Evidence: evidence,
	}
}

// generationLag reports a controller that has not observed the latest
// spec for DefaultGenerationGrace. Entities without an observed
// generation (metadata-only watches, kinds without one) are skipped.
func generationLag(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	generation, ok := number(e, kube.AttrGeneration)
	if !ok || generation <= 0 {
		return detection.Finding{}, false
	}
	observed, ok := number(e, kube.AttrObservedGen)
	if !ok || observed >= generation {
		return detection.Finding{}, false
	}
	since := latest(valueSince(e, kube.AttrGeneration),
		valueSince(e, kube.AttrObservedGen))
	if !sustained(ctx, "generation-lag", since, DefaultGenerationGrace) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.GenerationLagging, Severity: detection.Warning,
		Health: detection.Degraded, Since: since,
		Summary: "Its controller has not processed the latest change",
		Evidence: []detection.Evidence{
			{Label: "generation", Value: generationText(generation)},
			{Label: "observed generation", Value: generationText(observed)},
		},
	}, true
}

// ownsDeletion are the kinds whose own detector judges a deletion that
// takes long.
var ownsDeletion = map[inventory.Kind]bool{
	kube.KindPod: true, kube.KindNode: true, kube.KindNamespace: true,
}

// stuckDeletion reports an object whose deletion finalizers have held
// for DefaultDeletionGrace.
func stuckDeletion(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	finalizers := splitList(text(e, kube.AttrFinalizers))
	if !flag(e, kube.AttrDeleting) || len(finalizers) == 0 {
		return detection.Finding{}, false
	}
	since := timestamp(e, kube.AttrDeletingSince)
	if since.IsZero() {
		since = valueSince(e, kube.AttrDeleting)
	}
	if !sustained(ctx, "stuck-deletion", since, DefaultDeletionGrace) {
		return detection.Finding{}, false
	}
	names := strings.Join(finalizers, ", ")
	summary := "Deletion is blocked by finalizers " + names
	evidence := []detection.Evidence{
		{Label: detection.EvidenceFinalizers, Value: names}}
	if users := claimUsers(ctx, e); users != "" {
		summary += " (still used by " + users + ")"
		evidence = append(evidence,
			detection.Evidence{Label: "still used by", Value: users})
	}
	if warning, ok := lastWarning(ctx, e); ok {
		evidence = append(evidence, detection.Evidence{
			Label: detection.EvidenceLastWarning, Value: warning})
	}
	severity := detection.Warning
	if leftover(ctx, e, since) {
		severity = detection.Info
	}
	return detection.Finding{
		Reason: reasons.StuckDeleting, Severity: severity,
		Health: detection.Degraded, Since: since,
		Summary: summary, Evidence: evidence,
	}, true
}

// failedPhases are status phases that mean the object does not work.
var failedPhases = map[string]bool{
	"Failed": true, "Error": true, "Lost": true,
}

// badPhase reports a failed phase at once and Pending after
// DefaultPendingGrace. A run that finished failed (a Workflow, a
// PipelineRun: the object records when its run ended) is news when it
// fails; once it finished staleRunAge ago it will not change by itself
// and goes to the digest, the way a failed Job does. A long-lived object
// in a failed phase has no finish time and stays a warning.
func badPhase(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	phase := text(e, kube.AttrPhase)
	since := valueSince(e, kube.AttrPhase)
	evidence := []detection.Evidence{{Label: "phase", Value: phase}}
	if message := text(e, kube.AttrMessage); message != "" {
		evidence = append(evidence,
			detection.Evidence{Label: "message", Value: message})
	}
	switch {
	case failedPhases[phase]:
		severity := detection.Warning
		if finished := timestamp(e, kube.AttrFinished); !finished.IsZero() {
			severity = failedJobSeverity(ctx, finished)
		}
		return detection.Finding{
			Reason: reasons.PhaseFailed, Severity: severity,
			Health: detection.Failing, Since: since,
			Summary: "Reports phase " + phase, Evidence: evidence,
		}, true
	case phase == "Pending" &&
		sustained(ctx, "phase-pending", since, DefaultPendingGrace):
		return detection.Finding{
			Reason: reasons.PhasePending, Severity: detection.Warning,
			Health: detection.Degraded, Since: since,
			Summary: "Has been pending", Evidence: evidence,
		}, true
	}
	return detection.Finding{}, false
}

func generationText(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func splitList(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}
