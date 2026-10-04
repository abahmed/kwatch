package detectors

import (
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
	for _, conditionType := range []string{
		"ControllerResizeError", "NodeResizeError",
	} {
		if status, reason, since := condition(e, conditionType); status ==
			"True" {
			out = append(out, detection.Finding{
				Reason:   reasons.PersistentVolumeClaim,
				Severity: detection.Warning, Since: since,
				Summary: "Volume resize failed (" + reason + ")",
				Evidence: []detection.Evidence{{
					Label: "message",
					Value: conditionMessage(e, conditionType),
				}},
			})
			break
		}
	}
	return out
}

// awaitingFirstConsumer reports a claim of a WaitForFirstConsumer class
// that no pod mounts yet. Such a claim stays Pending by design until its
// first pod is scheduled.
func awaitingFirstConsumer(ctx detection.Context, e inventory.Entity) bool {
	if ctx.Model == nil {
		return false
	}
	consumers := ctx.Model.Related(e.ID, inventory.Mounts, inventory.Incoming)
	if len(consumers) > 0 {
		return false
	}
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
func unusedClaim(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	if ctx.Model == nil || !ctx.Synced(kube.KindPod) || len(ctx.Model.Related(
		e.ID, inventory.Mounts, inventory.Incoming)) > 0 {
		return detection.Finding{}, false
	}
	since := ctx.Onset("unused", valueSince(e, kube.AttrPhase))
	if !sustained(ctx, "unused", since, DefaultUnusedAfter) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.ClaimUnused, Severity: detection.Info, Since: since,
		Summary: "Volume claim is bound but no pod has mounted it for " +
			format.Duration(ctx.Now.Sub(since)),
	}, true
}
