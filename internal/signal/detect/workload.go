package detect

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultUnavailable is how long a workload may run below its desired
// replicas before it is a signal. Rollouts normally finish sooner.
const DefaultUnavailable = 5 * time.Minute

// Workload detects controllers that cannot keep their pods available:
// stalled rollouts, sustained unavailability and replica failures.
type Workload struct {
	unavailable time.Duration
}

// NewWorkload builds the workload detector.
func NewWorkload(unavailable time.Duration) Workload {
	if unavailable <= 0 {
		unavailable = DefaultUnavailable
	}
	return Workload{unavailable: unavailable}
}

// Name implements signal.Detector.
func (Workload) Name() string { return "workload" }

// Kinds implements signal.Detector.
func (Workload) Kinds() []knowledge.Kind {
	return []knowledge.Kind{
		kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
	}
}

// Detect implements signal.Detector.
func (d Workload) Detect(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	var out []signal.Signal
	if _, reason, since := condition(e, "Progressing"); reason ==
		constant.ReasonProgressDeadlineExceeded {
		out = append(out, signal.Signal{
			Reason:   constant.ReasonProgressDeadlineExceeded,
			Severity: signal.Critical, Since: since,
			Summary: "Rollout is stuck: new pods did not become available " +
				"within the progress deadline",
		})
	}
	if status, reason, since := condition(e, "ReplicaFailure"); status ==
		"True" {
		out = append(out, signal.Signal{
			Reason:   constant.ReasonDeploymentReplicaFailure,
			Severity: signal.Critical, Since: since,
			Summary: "Controller cannot create pods (" + reason + ")",
			Evidence: []signal.Evidence{{
				Label: "message", Value: conditionMessage(e, "ReplicaFailure"),
			}},
		})
	}
	if s, ok := d.availability(ctx, e); ok {
		out = append(out, s)
	}
	return out
}

func (d Workload) availability(
	ctx signal.Context, e knowledge.Entity,
) (signal.Signal, bool) {
	desired, ok := desiredReplicas(e)
	if !ok || desired == 0 {
		return signal.Signal{}, false
	}
	ready, _ := number(e, kube.AttrReadyReplicas)
	if ready >= desired {
		return signal.Signal{}, false
	}
	since := valueSince(e, kube.AttrReadyReplicas)
	if !sustained(ctx, since, d.unavailable) {
		return signal.Signal{}, false
	}
	return signal.Signal{
		Reason: unavailableReason(e.ID.Kind), Severity: severityFor(
			ready, desired),
		Since: since, Symptom: true,
		Summary: strconv.Itoa(int(ready)) + " of " +
			strconv.Itoa(int(desired)) + " replicas are ready",
	}, true
}

func desiredReplicas(e knowledge.Entity) (float64, bool) {
	if e.ID.Kind == kube.KindDaemonSet {
		return number(e, kube.AttrDesiredReplicas)
	}
	return number(e, kube.AttrReplicas)
}

func unavailableReason(kind knowledge.Kind) string {
	switch kind {
	case kube.KindStatefulSet:
		return constant.ReasonStsUnavailable
	case kube.KindDaemonSet:
		return constant.ReasonDaemonSetUnavailable
	default:
		return constant.ReasonDeploymentUnavailable
	}
}

// severityFor is critical when nothing is ready, a warning otherwise.
func severityFor(ready, desired float64) signal.Severity {
	if ready == 0 && desired > 0 {
		return signal.Critical
	}
	return signal.Warning
}

// Job detects failed Jobs.
type Job struct{}

// Name implements signal.Detector.
func (Job) Name() string { return "job" }

// Kinds implements signal.Detector.
func (Job) Kinds() []knowledge.Kind { return []knowledge.Kind{kube.KindJob} }

// Detect implements signal.Detector.
func (Job) Detect(_ signal.Context, e knowledge.Entity) []signal.Signal {
	status, reason, since := condition(e, "Failed")
	if status != "True" {
		return nil
	}
	signalReason := constant.ReasonJobFailed
	switch reason {
	case "BackoffLimitExceeded":
		signalReason = constant.ReasonJobBackoffLimitExceeded
	case constant.ReasonDeadlineExceeded:
		signalReason = constant.ReasonJobDeadlineExceeded
	}
	failed, _ := number(e, kube.AttrFailed)
	return []signal.Signal{{
		Reason: signalReason, Severity: signal.Warning, Since: since,
		Summary: "Job failed after " + strconv.Itoa(int(failed)) +
			" failed pod(s)",
		Evidence: []signal.Evidence{{
			Label: "message", Value: conditionMessage(e, "Failed"),
		}},
	}}
}

// HPA detects autoscalers that cannot compute or apply a scale.
type HPA struct{}

// Name implements signal.Detector.
func (HPA) Name() string { return "hpa" }

// Kinds implements signal.Detector.
func (HPA) Kinds() []knowledge.Kind { return []knowledge.Kind{kube.KindHPA} }

// Detect implements signal.Detector.
func (HPA) Detect(_ signal.Context, e knowledge.Entity) []signal.Signal {
	var out []signal.Signal
	if status, reason, since := condition(e, "ScalingActive"); status ==
		"False" && reason != "ScalingDisabled" {
		out = append(out, signal.Signal{
			Reason:   constant.ReasonFailedGetResourceMetric,
			Severity: signal.Warning, Since: since,
			Summary: "Autoscaler cannot read the metrics it scales on",
			Evidence: []signal.Evidence{{
				Label: "message",
				Value: conditionMessage(e, "ScalingActive"),
			}},
		})
	}
	if status, _, since := condition(e, "AbleToScale"); status == "False" {
		out = append(out, signal.Signal{
			Reason: constant.ReasonHPAScalingError, Severity: signal.Warning,
			Since: since, Summary: "Autoscaler cannot change the replicas",
		})
	}
	current, _ := number(e, kube.AttrCurrentReplicas)
	desired, _ := number(e, kube.AttrDesiredReplicas)
	maximum, ok := number(e, kube.AttrMaxReplicas)
	if ok && current >= maximum && desired >= maximum {
		if status, _, since := condition(e, "ScalingLimited"); status ==
			"True" {
			out = append(out, signal.Signal{
				Reason: constant.ReasonHPAMaxedOut, Severity: signal.Warning,
				Since: since,
				Summary: "Autoscaler is at its maximum of " +
					strconv.Itoa(int(maximum)) + " replicas and wants more",
			})
		}
	}
	return out
}
