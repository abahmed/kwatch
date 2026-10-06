package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// PodThresholds bounds how long a pod may stay in a state before it is a
// finding. Zero values take the defaults.
type PodThresholds struct {
	NotReady    time.Duration
	Pending     time.Duration
	Terminating time.Duration
}

// Default pod thresholds.
const (
	DefaultNotReady    = 3 * time.Minute
	DefaultPending     = 2 * time.Minute
	DefaultTerminating = 10 * time.Minute
	maxStartupBudget   = 15 * time.Minute
)

// gatedPendingFactor stretches the pending threshold for a pod held by
// scheduling gates. A gate is held on purpose by the controller that
// added it, often while it waits for capacity or quota, so only a gate
// held five times as long as an ordinary pending pod looks forgotten.
const gatedPendingFactor = 5

// startedPendingFactor stretches the pending threshold for a pod that is
// scheduled but has not started. Its container findings already report
// image and configuration problems, so the pod itself only reports a
// wait three times as long as an ordinary pending pod.
const startedPendingFactor = 3

// Pod detects pod-level failures: unschedulable, evicted, failed, stuck
// terminating, and not ready for longer than the pod's own startup budget.
type Pod struct {
	thresholds PodThresholds
}

// NewPod builds the pod detector.
func NewPod(thresholds PodThresholds) Pod {
	if thresholds.NotReady <= 0 {
		thresholds.NotReady = DefaultNotReady
	}
	if thresholds.Pending <= 0 {
		thresholds.Pending = DefaultPending
	}
	if thresholds.Terminating <= 0 {
		thresholds.Terminating = DefaultTerminating
	}
	return Pod{thresholds: thresholds}
}

// Name implements detection.Detector.
func (Pod) Name() string { return "pod" }

// Kinds implements detection.Detector.
func (Pod) Kinds() []inventory.Kind { return []inventory.Kind{kube.KindPod} }

// Detect implements detection.Detector.
func (d Pod) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if flag(e, kube.AttrDeleting) {
		return append(d.terminating(ctx, e), preemptionFindings(ctx, e)...)
	}
	switch text(e, kube.AttrPhase) {
	case "Pending":
		return d.pending(ctx, e)
	case "Failed":
		return failed(ctx, e)
	case "Running":
		return append(d.notReady(ctx, e), resizeFindings(ctx, e)...)
	case "Unknown":
		return []detection.Finding{{
			Reason:   reasons.PodStatusUnknown,
			Severity: detection.Warning,
			Since:    valueSince(e, kube.AttrPhase),
			Summary:  "Pod state is unknown: its node stopped reporting",
		}}
	default:
		return nil
	}
}

func (d Pod) terminating(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	requested, deadline := deletionTimes(e)
	if !sustained(ctx, "pod-terminating", deadline, d.thresholds.Terminating) {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.PodStuckTerminating, Severity: detection.Warning,
		Since: requested,
		Summary: "Pod has been terminating for " +
			format.Duration(ctx.Now.Sub(requested)),
	}}
}

// deletionTimes returns when the pod's deletion was requested and when it
// was due to be over. The API server sets deletionTimestamp to the request
// plus the grace period, so a pod is only stuck once that deadline has
// passed by the threshold. A pod without the attributes (seen before they
// existed) counts from the moment kwatch saw it deleting.
func deletionTimes(e inventory.Entity) (requested, deadline time.Time) {
	deadline = timestamp(e, kube.AttrDeletionTime)
	if deadline.IsZero() {
		seen := valueSince(e, kube.AttrDeleting)
		return seen, seen
	}
	grace, _ := number(e, kube.AttrTerminationGrace)
	return deadline.Add(-time.Duration(grace) * time.Second), deadline
}

func (d Pod) pending(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	status, reason, since := condition(e, "PodScheduled")
	if status == "False" && reason == reasons.SchedulingGated {
		gated := d.thresholds.Pending * gatedPendingFactor
		if !sustained(ctx, "pod-gated", since, gated) {
			return nil
		}
		return []detection.Finding{{
			Reason: reasons.SchedulingGated, Severity: detection.Warning,
			Since: since,
			Summary: "Pod is held by scheduling gates that were never " +
				"removed",
		}}
	}
	if status == "False" {
		// While a pool boots, capacity for the pod is on its way.
		wait := d.thresholds.Pending + bootGraceFor(ctx, e) +
			scaleUpGrace(ctx, e)
		if !sustained(ctx, "pod-unschedulable", since, wait) {
			return nil
		}
		return []detection.Finding{{
			Reason: reasons.Unschedulable, Severity: detection.Warning,
			Since: since,
			Summary: "Pod cannot be scheduled (" + reason + ") for " +
				format.Duration(ctx.Now.Sub(since)),
			Evidence: unschedulableEvidence(ctx, e),
		}}
	}
	if status == "" {
		return d.unscheduled(ctx, e)
	}
	// Scheduled but not started: container-level findings explain image
	// and configuration incidents, so only a long wait is reported here.
	started := timestamp(e, kube.AttrStartTime)
	if started.IsZero() {
		// No start time yet: count from when the phase was first seen.
		started = valueSince(e, kube.AttrPhase)
	}
	wait := d.thresholds.Pending*startedPendingFactor +
		bootGraceFor(ctx, e)
	if !sustained(ctx, "pod-pending", started, wait) {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.PodPending, Severity: detection.Warning,
		Since:   started,
		Summary: "Pod has been Pending since it was scheduled",
	}}
}

// unscheduled reports a pending pod without any scheduling condition: no
// scheduler has looked at it, which is how a stopped scheduler shows.
func (d Pod) unscheduled(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	created := timestamp(e, kube.AttrCreated)
	wait := d.thresholds.Pending + bootGraceFor(ctx, e)
	if created.IsZero() ||
		!sustained(ctx, "pod-unscheduled", created, wait) {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.PodPending, Severity: detection.Warning,
		Since: created,
		Summary: "Pod has not been looked at by the scheduler for " +
			format.Duration(ctx.Now.Sub(created)),
	}}
}

func failed(ctx detection.Context, e inventory.Entity) []detection.Finding {
	evicted := text(e, kube.AttrReason) == reasons.Evicted
	if evicted && evictionOver(ctx, valueSince(e, kube.AttrPhase)) {
		return nil
	}
	if disrupted(e) && !(evicted && nodePressureEviction(e)) {
		return nil
	}
	if s, ok := admissionFinding(e); ok {
		return []detection.Finding{s}
	}
	findingReason, summary := reasons.PodFailed, "Pod failed"
	if evicted {
		findingReason, summary = reasons.Evicted, "Pod was evicted"
	}
	return []detection.Finding{{
		Reason: findingReason, Severity: detection.Warning,
		Since:   valueSince(e, kube.AttrPhase),
		Summary: summary,
		Evidence: []detection.Evidence{{
			Label: "message", Value: text(e, kube.AttrMessage),
		}},
	}}
}

// notReady reports a running pod that stays unready. A pod that has never
// been ready gets the startup budget its own probes declare.
func (d Pod) notReady(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if flag(e, kube.AttrReady) {
		return nil
	}
	since := timestamp(e, kube.AttrReadySince)
	threshold := d.thresholds.NotReady
	if budget := startupBudget(ctx, e.ID); budget > threshold {
		threshold = budget
	}
	threshold += replacementGraceFor(ctx, e)
	if !sustained(ctx, "pod-not-ready", since, threshold) {
		return nil
	}
	s := detection.Finding{
		Reason: reasons.ContainersNotReady, Severity: detection.Warning,
		Since: since,
		Summary: "Pod has not been ready for " +
			format.Duration(ctx.Now.Sub(since)),
	}
	// A start inside what the workload's pods usually need is the
	// usual start, a slow one that the digest can carry.
	readyJudgement(ctx, e, since).apply(&s)
	return []detection.Finding{s}
}

func startupBudget(
	ctx detection.Context, pod inventory.EntityID,
) time.Duration {
	var longest time.Duration
	for _, id := range ctx.Model.Related(
		pod, inventory.PartOf, inventory.Incoming,
	) {
		container, ok := ctx.Model.Entity(id)
		if !ok {
			continue
		}
		if seconds, ok := number(container, kube.AttrProbeBudget); ok {
			if budget := time.Duration(seconds) * time.Second; budget > longest {
				longest = budget
			}
		}
	}
	return min(longest, maxStartupBudget)
}

func conditionMessage(e inventory.Entity, conditionType string) string {
	return text(e, kube.ConditionKey(conditionType)+
		kube.AttrConditionMessage)
}

// disrupted reports a pod Kubernetes is terminating on purpose: the
// DisruptionTarget condition marks preemption, API eviction, taint
// manager deletion, pod GC, graceful node shutdown and scale-down. Such
// a pod ends Failed without having failed.
func disrupted(e inventory.Entity) bool {
	status, _, _ := condition(e, "DisruptionTarget")
	return status == "True"
}

// nodePressureEviction reports the kubelet evicting a pod for node
// pressure (status reason Evicted; a graceful node shutdown uses reason
// Terminated instead). It is a real symptom of the node's condition, so
// it stays visible and is attributed to the node.
func nodePressureEviction(e inventory.Entity) bool {
	_, reason, _ := condition(e, "DisruptionTarget")
	return reason == "TerminationByKubelet"
}

// unschedulableEvidence is the scheduler's message plus, when it names
// a CPU or memory shortage, the numbers behind it.
func unschedulableEvidence(
	ctx detection.Context, e inventory.Entity,
) []detection.Evidence {
	message := conditionMessage(e, "PodScheduled")
	out := []detection.Evidence{{Label: "scheduler", Value: message}}
	out = append(out, capacityEvidence(ctx.Model, e.ID, message)...)
	return append(out, fitEvidence(ctx, e)...)
}

// evictionOver reports an eviction older than EventWindow. The evicted
// pod stays in the API until pod garbage collection, but the eviction
// happened once: like a Warning event, it stops counting as current when
// the window has passed, and a healthy replacement has long since taken
// over. Otherwise old evicted pods would be announced as new problems
// when the node's own events age out.
func evictionOver(ctx detection.Context, since time.Time) bool {
	if since.IsZero() {
		return false
	}
	left := since.Add(EventWindow).Sub(ctx.Now)
	if left <= 0 {
		return true
	}
	ctx.RecheckAfter(left + time.Nanosecond)
	return false
}
