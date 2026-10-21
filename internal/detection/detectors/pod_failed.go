package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// failed reports a pod in the Failed phase: an eviction, an admission
// rejection or a run that ended in error. Kubernetes keeps such a pod in
// the API long after it failed, so its age decides how loud it is.
func failed(ctx detection.Context, e inventory.Entity) []detection.Finding {
	since := podFailedAt(ctx, e)
	evicted := text(e, kube.AttrReason) == reasons.Evicted
	if evicted && evictionOver(ctx, since) {
		return nil
	}
	if disrupted(e) && !(evicted && nodePressureEviction(e)) ||
		daemonPodOnLeavingNode(ctx, e) {
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
		Reason: findingReason, Severity: failedPodSeverity(ctx, since),
		Since: since, Summary: summary,
		Advisory: jobRetrying(ctx, e),
		Evidence: []detection.Evidence{{
			Label: "message", Value: text(e, kube.AttrMessage),
		}},
	}}
}

// podFailedAt is when the pod's last container ended: the cluster's own
// record, which stays true after kwatch restarts. A pod without a
// finished container (rejected before it started) counts from when its
// phase was first seen.
func podFailedAt(ctx detection.Context, pod inventory.Entity) time.Time {
	var at time.Time
	if ctx.Model != nil {
		for _, id := range ctx.Model.Related(pod.ID, inventory.PartOf,
			inventory.Incoming) {
			if container, ok := ctx.Model.Entity(id); ok {
				if t := timestamp(container, kube.AttrFinished); t.After(at) {
					at = t
				}
			}
		}
	}
	if at.IsZero() {
		return valueSince(pod, kube.AttrPhase)
	}
	return at
}

// failedPodSeverity is a warning while the failure is news and
// informational once EventWindow has passed: it is then history that
// the digest carries, as an old Warning event stops counting.
func failedPodSeverity(
	ctx detection.Context, since time.Time,
) detection.Severity {
	if since.IsZero() || ageOut(ctx, since) {
		return detection.Info
	}
	return detection.Warning
}

// ageOut reports a failure older than EventWindow. When it is not yet,
// it asks to be evaluated again as the window closes.
func ageOut(ctx detection.Context, since time.Time) bool {
	left := since.Add(EventWindow).Sub(ctx.Now)
	if left <= 0 {
		return true
	}
	ctx.RecheckAfter(left + time.Nanosecond)
	return false
}

// evictionOver reports an eviction older than EventWindow. The evicted
// pod stays in the API until pod garbage collection, but the eviction
// happened once: like a Warning event, it stops counting as current when
// the window has passed, and a healthy replacement has long since taken
// over. Otherwise old evicted pods would be announced as new problems
// when the node's own events age out.
func evictionOver(ctx detection.Context, since time.Time) bool {
	return !since.IsZero() && ageOut(ctx, since)
}

// daemonPodOnLeavingNode reports a pod of a DaemonSet that failed on a
// node that is leaving, not ready, cordoned, gone or just joined: the
// pod is part of its node, so the node's lifecycle ends it, as when an
// autoscaler consolidates the node.
func daemonPodOnLeavingNode(
	ctx detection.Context, pod inventory.Entity,
) bool {
	if ctx.Model == nil {
		return false
	}
	owned := false
	for _, id := range ctx.Model.Related(pod.ID, inventory.OwnedBy,
		inventory.Outgoing) {
		owned = owned || id.Kind == kube.KindDaemonSet
	}
	nodes := ctx.Model.Related(pod.ID, inventory.RunsOn, inventory.Outgoing)
	if !owned || len(nodes) == 0 {
		return false
	}
	for _, node := range nodes {
		if !nodeInTransition(ctx, node) {
			return false
		}
	}
	return true
}
