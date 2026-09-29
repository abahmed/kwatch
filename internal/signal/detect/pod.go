package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// PodThresholds bounds how long a pod may stay in a state before it is a
// signal. Zero values take the defaults.
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

// Name implements signal.Detector.
func (Pod) Name() string { return "pod" }

// Kinds implements signal.Detector.
func (Pod) Kinds() []knowledge.Kind { return []knowledge.Kind{kube.KindPod} }

// Detect implements signal.Detector.
func (d Pod) Detect(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	if flag(e, kube.AttrDeleting) {
		return d.terminating(ctx, e)
	}
	switch text(e, kube.AttrPhase) {
	case "Pending":
		return d.pending(ctx, e)
	case "Failed":
		return failed(e)
	case "Running":
		return d.notReady(ctx, e)
	case "Unknown":
		return []signal.Signal{{
			Reason:   constant.ReasonPodStatusUnknown,
			Severity: signal.Warning,
			Since:    valueSince(e, kube.AttrPhase),
			Summary:  "Pod state is unknown: its node stopped reporting",
		}}
	default:
		return nil
	}
}

func (d Pod) terminating(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	since := valueSince(e, kube.AttrDeleting)
	if !sustained(ctx, since, d.thresholds.Terminating) {
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonPodStuckTerminating, Severity: signal.Warning,
		Since: since,
		Summary: "Pod has been terminating for " +
			format.Duration(ctx.Now.Sub(since)),
	}}
}

func (d Pod) pending(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	status, reason, since := condition(e, "PodScheduled")
	if status == "False" && reason == constant.ReasonSchedulingGated {
		if !sustained(ctx, since, d.thresholds.Pending*5) {
			return nil
		}
		return []signal.Signal{{
			Reason: constant.ReasonSchedulingGated, Severity: signal.Warning,
			Since: since,
			Summary: "Pod is held by scheduling gates that were never " +
				"removed",
		}}
	}
	if status == "False" {
		if !sustained(ctx, since, d.thresholds.Pending) {
			return nil
		}
		return []signal.Signal{{
			Reason: constant.ReasonUnschedulable, Severity: signal.Warning,
			Since: since,
			Summary: "Pod cannot be scheduled (" + reason + ") for " +
				format.Duration(ctx.Now.Sub(since)),
			Evidence: []signal.Evidence{{
				Label: "scheduler",
				Value: conditionMessage(e, "PodScheduled"),
			}},
		}}
	}
	// Scheduled but not started: container-level signals explain image
	// and configuration problems, so only a long wait is reported here.
	started := timestamp(e, kube.AttrStartTime)
	if !sustained(ctx, started, d.thresholds.Pending*3) {
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonPodPending, Severity: signal.Warning,
		Since:   started,
		Summary: "Pod has been Pending since it was scheduled",
	}}
}

func failed(e knowledge.Entity) []signal.Signal {
	signalReason, summary := constant.ReasonPodFailed, "Pod failed"
	if text(e, kube.AttrReason) == constant.ReasonEvicted {
		signalReason, summary = constant.ReasonEvicted, "Pod was evicted"
	}
	return []signal.Signal{{
		Reason: signalReason, Severity: signal.Warning,
		Since:   valueSince(e, kube.AttrPhase),
		Summary: summary,
		Evidence: []signal.Evidence{{
			Label: "message", Value: text(e, kube.AttrMessage),
		}},
	}}
}

// notReady reports a running pod that stays unready. A pod that has never
// been ready gets the startup budget its own probes declare.
func (d Pod) notReady(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	if flag(e, kube.AttrReady) {
		return nil
	}
	since := timestamp(e, kube.AttrReadySince)
	threshold := d.thresholds.NotReady
	if budget := startupBudget(ctx, e.ID); budget > threshold {
		threshold = budget
	}
	if !sustained(ctx, since, threshold) {
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonContainersNotReady, Severity: signal.Warning,
		Since: since,
		Summary: "Pod has not been ready for " +
			format.Duration(ctx.Now.Sub(since)),
	}}
}

func startupBudget(ctx signal.Context, pod knowledge.EntityID) time.Duration {
	var longest time.Duration
	for _, id := range ctx.Model.Related(
		pod, knowledge.PartOf, knowledge.Incoming,
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

func conditionMessage(e knowledge.Entity, conditionType string) string {
	return text(e, kube.ConditionKey(conditionType)+
		kube.AttrConditionMessage)
}
