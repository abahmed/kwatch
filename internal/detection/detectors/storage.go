package detectors

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultClaimPending is how long a claim may wait for a volume. Dynamic
// provisioning normally binds within seconds.
const DefaultClaimPending = 2 * time.Minute

const waitForFirstConsumer = "WaitForFirstConsumer"

// Claim detects PersistentVolumeClaims that cannot bind, were lost, or
// cannot finish a resize.
type Claim struct{}

// Name implements detection.Detector.
func (Claim) Name() string { return "claim" }

// Kinds implements detection.Detector.
func (Claim) Kinds() []inventory.Kind { return []inventory.Kind{kube.KindPVC} }

// Detect implements detection.Detector.
func (Claim) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	switch text(e, kube.AttrPhase) {
	case "Pending":
		since := valueSince(e, kube.AttrPhase)
		if sustained(ctx, "claim-pending", since, DefaultClaimPending) &&
			!awaitingFirstConsumer(ctx, e) {
			out = append(out, detection.Finding{
				Reason:   reasons.PersistentVolumeClaim,
				Severity: detection.Warning, Since: since,
				Summary: "Volume claim has been waiting for a volume for " +
					format.Duration(ctx.Now.Sub(since)),
				Evidence: provisionerEvidence(ctx, e),
			})
		}
	case "Lost":
		out = append(out, detection.Finding{
			Reason:   reasons.PersistentVolumeClaim,
			Severity: detection.Critical,
			Since:    valueSince(e, kube.AttrPhase),
			Summary:  "Volume claim lost its bound volume",
		})
	case "Bound":
		if s, ok := unusedClaim(ctx, e); ok {
			out = append(out, s)
		}
	}
	// A finding is keyed by entity and reason, so a claim reports one
	// finding: the phase problem first, else the first resize failure.
	if len(out) > 0 {
		return out
	}
	return conditionFindings(ctx, e)
}

// awaitingFirstConsumer reports a claim of a WaitForFirstConsumer class
// whose pods are not scheduled yet: none mounts it, or every one that
// does still waits for a node (an autoscaler adding one, say). Such a
// claim stays Pending by design until its first pod is scheduled; an
// unschedulable pod is reported on its own.
func awaitingFirstConsumer(ctx detection.Context, e inventory.Entity) bool {
	if ctx.Model == nil || !firstConsumerClass(ctx, e) {
		return false
	}
	for _, id := range ctx.Model.Related(e.ID, inventory.Mounts,
		inventory.Incoming) {
		if pod, ok := ctx.Model.Entity(id); !ok || podScheduled(ctx, pod) {
			return false
		}
	}
	return true
}

// firstConsumerClass reports a claim whose storage class binds on the
// first consumer.
func firstConsumerClass(ctx detection.Context, e inventory.Entity) bool {
	for _, id := range ctx.Model.Related(
		e.ID, inventory.References, inventory.Outgoing,
	) {
		if id.Kind != kube.KindStorageClass {
			continue
		}
		class, ok := ctx.Model.Entity(id)
		if ok && text(class, kube.AttrBindingMode) == waitForFirstConsumer {
			return true
		}
	}
	return false
}

// podScheduled reports a pod the scheduler placed on a node.
func podScheduled(ctx detection.Context, pod inventory.Entity) bool {
	if text(pod, kube.ConditionKey("PodScheduled")) == "True" {
		return true
	}
	return len(ctx.Model.Related(pod.ID, inventory.RunsOn,
		inventory.Outgoing)) > 0
}

// Volume detects failed PersistentVolumes.
type Volume struct{}

// Name implements detection.Detector.
func (Volume) Name() string { return "volume" }

// Kinds implements detection.Detector.
func (Volume) Kinds() []inventory.Kind { return []inventory.Kind{kube.KindPV} }

// Detect implements detection.Detector.
func (Volume) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if text(e, kube.AttrPhase) != "Failed" {
		return nil
	}
	evidence := []detection.Evidence{{
		Label: "message", Value: text(e, kube.AttrMessage),
	}}
	return []detection.Finding{{
		Reason: reasons.PersistentVolume, Severity: detection.Critical,
		Since:    valueSince(e, kube.AttrPhase),
		Summary:  "Persistent volume failed (" + text(e, kube.AttrReason) + ")",
		Evidence: append(evidence, reclaimEvents(ctx, e.ID)...),
	}}
}

// reclaimEventReasons are the PV controller's Warning events for a failed
// reclaim; the volume's Failed phase is the finding, their messages (the
// provisioner's or recycler's error) are its evidence.
var reclaimEventReasons = map[string]bool{
	"VolumeFailedDelete": true, "VolumeFailedRecycle": true,
}

func reclaimEvents(
	ctx detection.Context, id inventory.EntityID,
) []detection.Evidence {
	if ctx.Model == nil {
		return nil
	}
	var out []detection.Evidence
	for _, note := range ctx.Model.Notes(id, ctx.Now.Add(-EventWindow)) {
		if note.Warning && reclaimEventReasons[note.Reason] {
			out = append(out, detection.Evidence{
				Label: note.Reason, Value: note.Message,
			})
		}
	}
	return out
}

// unusedClaim reports a bound claim no pod has mounted for a day: a
// volume that costs money and serves nothing. It waits for the digest.
// The day starts when kwatch first sees the claim without a mount, either
// when the last one goes or at a restart: how long it has been bound says
// nothing about its last use. A claim a CronJob's pods use is idle between
// runs by design.
func unusedClaim(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if ctx.Model == nil || !ctx.Synced(kube.KindPod) || claimInUse(ctx, e) {
		return detection.Finding{}, false
	}
	since := ctx.Onset("unused", ctx.Now)
	if !sustained(ctx, "unused", since, DefaultUnusedAfter) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.ClaimUnused, Severity: detection.Info, Since: since,
		// Clutter, not a failure: it only joins a real failure's incident.
		Advisory: true,
		Summary: "Volume claim is bound but no pod has mounted it for " +
			format.Duration(ctx.Now.Sub(since)),
	}, true
}

// claimInUse reports a claim a pod mounts, or one a workload's pod
// template names (a CronJob between runs, a workload scaled to zero),
// or a StatefulSet's claim (named <template>-<statefulset>-<ordinal>):
// such a claim is idle by design, not unused.
func claimInUse(ctx detection.Context, e inventory.Entity) bool {
	if len(ctx.Model.Related(
		e.ID, inventory.Mounts, inventory.Incoming)) > 0 {
		return true
	}
	for _, id := range ctx.Model.Related(
		e.ID, inventory.References, inventory.Incoming,
	) {
		if workloadKinds[id.Kind] {
			return true
		}
	}
	return statefulSetClaim(ctx, e.ID)
}

// workloadKinds are the kinds whose pod template can name a claim.
var workloadKinds = map[inventory.Kind]bool{
	kube.KindCronJob: true, kube.KindJob: true,
	kube.KindDeployment: true, kube.KindReplicaSet: true,
	kube.KindStatefulSet: true, kube.KindDaemonSet: true,
}

// statefulSetClaim reports a claim named like one of a StatefulSet of
// its namespace: <template>-<statefulset>-<ordinal>.
func statefulSetClaim(ctx detection.Context, claim inventory.EntityID) bool {
	head, ordinal, ok := strings.Cut(reverse(claim.Name), "-")
	if !ok || !allDigits(reverse(head)) {
		return false
	}
	base := reverse(ordinal)
	for _, sts := range ctx.Model.EntitiesIn(kube.KindStatefulSet,
		claim.Namespace) {
		if strings.HasSuffix(base, "-"+sts.Name) {
			return true
		}
	}
	return false
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}
