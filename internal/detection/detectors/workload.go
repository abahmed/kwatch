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

// DefaultUnavailable is how long a workload may run below its desired
// replicas before it is a finding. Rollouts normally finish sooner.
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

// Name implements detection.Detector.
func (Workload) Name() string { return "workload" }

// Kinds implements detection.Detector.
func (Workload) Kinds() []inventory.Kind {
	return []inventory.Kind{
		kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
		kube.KindReplicaSet,
	}
}

// Detect implements detection.Detector.
func (d Workload) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	if _, reason, since := condition(e, "Progressing"); reason ==
		reasons.ProgressDeadlineExceeded {
		out = append(out, detection.Finding{
			Reason:   reasons.ProgressDeadlineExceeded,
			Severity: detection.Critical, Since: since,
			Summary: "Rollout is stuck: new pods did not become available " +
				"within the progress deadline",
		})
	}
	if status, reason, since := condition(e, "ReplicaFailure"); status ==
		"True" {
		out = append(out, detection.Finding{
			Reason:   reasons.DeploymentReplicaFailure,
			Severity: detection.Critical, Since: since,
			Summary: "Controller cannot create pods (" + reason + ")",
			Evidence: []detection.Evidence{{
				Label: "message", Value: conditionMessage(e, "ReplicaFailure"),
			}},
		})
	}
	// A ReplicaSet's availability is its Deployment's; only its replica
	// failures (quota, admission) are its own.
	if e.ID.Kind == kube.KindReplicaSet {
		for i := range out {
			out[i].Reason = reasons.ReplicaSetFailure
		}
		return out
	}
	if s, ok := d.availability(ctx, e); ok {
		out = append(out, s)
	}
	if s, ok := neverReady(ctx, e); ok {
		out = append(out, s)
	}
	if s, ok := scaledToZeroRouted(ctx, e); ok {
		out = append(out, s)
	}
	if e.ID.Kind == kube.KindStatefulSet {
		if s, ok := statefulSetRollout(ctx, e); ok {
			out = append(out, s)
		}
	}
	return append(out, classReferences(ctx, e)...)
}

func (d Workload) availability(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	desired, ok := desiredReplicas(e)
	if !ok || desired == 0 {
		return detection.Finding{}, false
	}
	ready, _ := number(e, kube.AttrReadyReplicas)
	if ready >= desired {
		return detection.Finding{}, false
	}
	// The wait runs from when the workload first fell short, not from
	// the latest change of the ready count: losing one more replica
	// during an outage must not restart it.
	since := ctx.Onset("unavailable", latest(
		valueSince(e, kube.AttrReadyReplicas),
		valueSince(e, desiredAttr(e))))
	wait := d.unavailable + workloadReplacementGrace(ctx, e.ID)
	if !sustained(ctx, "unavailable", since, wait) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: unavailableReason(e.ID.Kind), Severity: severityFor(
			ready, desired),
		Since: since, Symptom: true,
		Summary: strconv.Itoa(int(ready)) + " of " +
			strconv.Itoa(int(desired)) + " replicas are ready",
		Evidence: append(daemonSetGaps(ctx.Model, e),
			neverHealthyEvidence(ctx, e)...),
	}, true
}

// maxGapNodes bounds how many nodes the DaemonSet evidence names.
const maxGapNodes = 3

// daemonSetGaps names, for a DaemonSet, the nodes whose pod is not
// ready and the taints on them, so the message says where the agent is
// missing instead of only how many. Other kinds get no evidence here.
func daemonSetGaps(
	model inventory.Reader, e inventory.Entity,
) []detection.Evidence {
	if e.ID.Kind != kube.KindDaemonSet || model == nil {
		return nil
	}
	var nodes, tainted []string
	for _, id := range model.Related(e.ID, inventory.OwnedBy,
		inventory.Incoming) {
		pod, ok := model.Entity(id)
		if !ok || id.Kind != kube.KindPod || flag(pod, kube.AttrReady) {
			continue
		}
		for _, node := range model.Related(id, inventory.RunsOn,
			inventory.Outgoing) {
			nodes = append(nodes, node.Name)
			if entity, ok := model.Entity(node); ok &&
				text(entity, kube.AttrTaints) != "" {
				tainted = append(tainted, node.Name+" ("+
					text(entity, kube.AttrTaints)+")")
			}
		}
	}
	if len(nodes) == 0 {
		return nil
	}
	sort.Strings(nodes)
	out := []detection.Evidence{{Label: "nodes without a ready pod",
		Value: nameList(nodes)}}
	if len(tainted) > 0 {
		sort.Strings(tainted)
		out = append(out, detection.Evidence{Label: "tainted nodes",
			Value: nameList(tainted)})
	}
	return out
}

// nameList joins up to maxGapNodes names and counts the rest.
func nameList(names []string) string {
	if len(names) <= maxGapNodes {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:maxGapNodes], ", ") + " and " +
		strconv.Itoa(len(names)-maxGapNodes) + " more"
}

func desiredReplicas(e inventory.Entity) (float64, bool) {
	return number(e, desiredAttr(e))
}

// desiredAttr names the attribute holding the desired replica count.
func desiredAttr(e inventory.Entity) string {
	if e.ID.Kind == kube.KindDaemonSet {
		return kube.AttrDesiredReplicas
	}
	return kube.AttrReplicas
}

func unavailableReason(kind inventory.Kind) string {
	switch kind {
	case kube.KindStatefulSet:
		return reasons.StsUnavailable
	case kube.KindDaemonSet:
		return reasons.DaemonSetUnavailable
	default:
		return reasons.DeploymentUnavailable
	}
}

// severityFor is critical when nothing is ready, a warning otherwise.
func severityFor(ready, desired float64) detection.Severity {
	if ready == 0 && desired > 0 {
		return detection.Critical
	}
	return detection.Warning
}

// Job detects failed Jobs.
type Job struct{}

// Name implements detection.Detector.
func (Job) Name() string { return "job" }

// Kinds implements detection.Detector.
func (Job) Kinds() []inventory.Kind { return []inventory.Kind{kube.KindJob} }

// Detect implements detection.Detector.
func (Job) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	out := classReferences(ctx, e)
	if f, ok := runningLong(ctx, e); ok {
		out = append(out, f)
	}
	status, reason, since := condition(e, "Failed")
	if status != "True" {
		return out
	}
	findingReason := reasons.JobFailed
	switch reason {
	case "BackoffLimitExceeded":
		findingReason = reasons.JobBackoffLimitExceeded
	case reasons.DeadlineExceeded:
		findingReason = reasons.JobDeadlineExceeded
	}
	failed, _ := number(e, kube.AttrFailed)
	return append(out, detection.Finding{
		Reason: findingReason, Severity: failedJobSeverity(ctx, e, since),
		Since: since,
		Summary: "Job failed after " + strconv.Itoa(int(failed)) +
			" failed pod(s)",
		Evidence: []detection.Evidence{{
			Label: "message", Value: conditionMessage(e, "Failed"),
		}},
	})
}
