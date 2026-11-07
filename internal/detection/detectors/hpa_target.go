package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// scalableKinds are the targets kwatch watches itself, so their absence
// from the model is conclusive. Any other target (a custom resource) is
// never concluded missing.
var scalableKinds = map[inventory.Kind]string{
	kube.KindDeployment:  "Deployment",
	kube.KindStatefulSet: "StatefulSet",
	kube.KindReplicaSet:  "ReplicaSet",
}

// missingScaleTarget reports an autoscaler whose scale target does not
// exist: it can never scale anything. It is a configuration error, not
// an outage, so it is informational and waits for the digest. The wait
// keeps a chart that creates the autoscaler before its Deployment quiet.
func missingScaleTarget(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if ctx.Model == nil {
		return detection.Finding{}, false
	}
	for _, target := range ctx.Model.Related(e.ID, inventory.Scales,
		inventory.Outgoing) {
		title, known := scalableKinds[target.Kind]
		if !known || !ctx.Synced(target.Kind) || ctx.Model.Exists(target) {
			continue
		}
		since := missingSince(ctx, e)
		if !sustained(ctx, "hpa-target-missing", since,
			DefaultConditionGrace) {
			return detection.Finding{}, false
		}
		return detection.Finding{
			Reason: reasons.HPATargetMissing, Severity: detection.Info,
			Since: since,
			Summary: "HPA " + e.ID.Namespace + "/" + e.ID.Name +
				" targets " + title + " " + target.Name +
				", which does not exist.",
			Evidence: scaleTargetEvidence(e),
		}, true
	}
	return detection.Finding{}, false
}

// scaleTargetEvidence quotes the controller's own message when it
// reported the target unreadable.
func scaleTargetEvidence(e inventory.Entity) []detection.Evidence {
	for _, conditionType := range []string{"AbleToScale", "ScalingActive"} {
		if _, reason, _ := condition(e, conditionType); reason ==
			"FailedGetScale" {
			return []detection.Evidence{{Label: "message",
				Value: conditionMessage(e, conditionType)}}
		}
	}
	return nil
}

// missingSince is when the controller first failed to read the target,
// or, without that condition, when kwatch first saw it missing.
func missingSince(ctx detection.Context, e inventory.Entity) time.Time {
	for _, conditionType := range []string{"AbleToScale", "ScalingActive"} {
		if _, reason, since := condition(e, conditionType); reason ==
			"FailedGetScale" && !since.IsZero() {
			return since
		}
	}
	return ctx.Onset("hpa-target-missing", ctx.Now)
}

// CeilingUnavailable is how long a workload may fall short of its ready
// replicas before it is a finding while its autoscaler sits at the
// maximum and wants more. The ceiling already names the reason the
// shortfall is not a transient, so the full DefaultUnavailable wait only
// delays the message.
const CeilingUnavailable = 3 * time.Minute

// atCeiling reports whether an autoscaler that scales id is at its
// maximum with ScalingLimited true.
func atCeiling(ctx detection.Context, id inventory.EntityID) bool {
	if ctx.Model == nil {
		return false
	}
	for _, hpa := range ctx.Model.Related(id, inventory.Scales,
		inventory.Incoming) {
		e, ok := ctx.Model.Entity(hpa)
		if !ok || hpa.Kind != kube.KindHPA {
			continue
		}
		current, _ := number(e, kube.AttrCurrentReplicas)
		desired, _ := number(e, kube.AttrDesiredReplicas)
		maximum, known := number(e, kube.AttrMaxReplicas)
		status, _, _ := condition(e, "ScalingLimited")
		if known && current >= maximum && desired >= maximum &&
			status == "True" {
			return true
		}
	}
	return false
}
