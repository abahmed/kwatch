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

// quotaNearLimit reports resources used to 80% or more of their hard
// limit: the next rollout or scale-up may be rejected.
func quotaNearLimit(e inventory.Entity) []detection.Finding {
	near := text(e, kube.AttrQuotaNearLimit)
	if near == "" {
		return nil
	}
	items := strings.Split(near, ",")
	evidence := make([]detection.Evidence, 0, len(items))
	for _, item := range items {
		evidence = append(evidence,
			detection.Evidence{Label: "used", Value: item})
	}
	return []detection.Finding{{
		Reason:   reasons.ResourceQuotaNearLimit,
		Severity: detection.Warning, Health: detection.Degraded,
		Since: valueSince(e, kube.AttrQuotaNearLimit),
		Summary: "Namespace quota is nearly used up (" +
			strings.Join(items, ", ") + ")",
		Evidence: evidence,
	}}
}

// quotaCreators are the kinds whose controllers record FailedCreate when
// the API server refuses the pods they ask for.
var quotaCreators = []inventory.Kind{
	kube.KindReplicaSet, kube.KindStatefulSet, kube.KindDaemonSet,
	kube.KindJob,
}

// quotaRefusalText is what the API server says when a quota refuses a
// create: "... is forbidden: exceeded quota: NAME, requested: ...".
const quotaRefusalText = "exceeded quota: "

// IsQuotaRefusal reports whether an event on a controller says the API
// server refused its create because a quota would be exceeded. The engine
// uses it to re-check the namespace quotas when such an event arrives,
// because the event lands on the controller and not on the quota.
func IsQuotaRefusal(note inventory.Note) bool {
	return note.Reason == "FailedCreate" &&
		strings.Contains(note.Message, quotaRefusalText)
}

// quotaRefusedCreate reports whether a controller in the namespace was
// recently refused a create because it would exceed a quota. A non-empty
// quota name narrows this to refusals that name that quota.
func quotaRefusedCreate(
	ctx detection.Context, namespace, quota string,
) bool {
	if ctx.Model == nil {
		return false
	}
	since := ctx.Now.Add(-EventWindow)
	needle := quotaRefusalText
	if quota != "" {
		needle += quota + ","
	}
	for _, kind := range quotaCreators {
		for _, id := range ctx.Model.EntitiesIn(kind, namespace) {
			for _, note := range ctx.Model.Notes(id, since) {
				if IsQuotaRefusal(note) &&
					strings.Contains(note.Message, needle) {
					ctx.RecheckAfter(note.At.Add(EventWindow).
						Sub(ctx.Now) + time.Nanosecond)
					return true
				}
			}
		}
	}
	return false
}

// DefaultDetachStuck is how long a VolumeAttachment may take to detach;
// the attach-detach controller force-detaches after six minutes.
const DefaultDetachStuck = 10 * time.Minute

// detachFailure reports a volume that cannot be detached: the CSI driver
// returned a detach error, or deletion of the attachment has been pending
// for DefaultDetachStuck. Until it detaches, the volume cannot attach to
// another node. When the node or the volume the attachment names no
// longer exists, nothing will ever finish the detach: the summary says so.
func detachFailure(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	message := text(e, kube.AttrDetachError)
	deleting := flag(e, kube.AttrDeleting)
	if message == "" && !deleting {
		return nil
	}
	since := valueSince(e, kube.AttrDetachError)
	if deleting {
		if at := deletingSince(e); !at.IsZero() {
			since = at
		}
	}
	if message == "" &&
		!sustained(ctx, "detach-stuck", since, DefaultDetachStuck) {
		return nil
	}
	f := detection.Finding{
		Reason: reasons.VolumeDetachFailure, Severity: detection.Warning,
		Health: detection.Failing, Since: since,
		Summary: "Volume cannot be detached from its node",
	}
	if message != "" {
		f.Evidence = []detection.Evidence{
			{Label: "detach error", Value: message}}
	} else {
		f.Summary = "Volume has been detaching for " +
			format.Duration(ctx.Now.Sub(since))
	}
	if deleting {
		stuckDeleting(ctx, e, &f)
	}
	return []detection.Finding{f}
}

// deletingSince is when deletion of e was requested, or zero.
func deletingSince(e inventory.Entity) time.Time {
	since := timestamp(e, kube.AttrDeletingSince)
	if since.IsZero() {
		since = valueSince(e, kube.AttrDeleting)
	}
	return since
}

// stuckDeleting words a deleting attachment whose node or volume is gone.
// With the volume gone nothing can attach or detach it any more and no
// workload can ask for it, so the attachment is only housekeeping
// (Info), whether or not the node is gone too; with only the node gone
// the volume still cannot attach to another node, which stays a warning.
// The summary names the volume, the node and the driver: the
// attachment's own name is an opaque hash.
func stuckDeleting(
	ctx detection.Context, e inventory.Entity, f *detection.Finding,
) {
	node, nodeGone := goneRelated(ctx, e, inventory.RunsOn, kube.KindNode)
	pv, pvGone := goneRelated(ctx, e, inventory.References, kube.KindPV)
	subject := "Attachment of volume " + pv + " to node " + node
	driver := text(e, kube.AttrAttacher)
	if driver != "" {
		subject += " by " + driver
	}
	var gone string
	switch {
	case nodeGone && pvGone:
		gone = "volume and node"
		f.Summary = subject + " is stuck deleting; the volume " + pv +
			" it attaches and the node no longer exist"
	case pvGone:
		gone = "volume"
		f.Summary = subject + " is stuck deleting; the volume " + pv +
			" it attaches no longer exists"
	case nodeGone:
		gone = "node"
		f.Summary = subject + " is stuck deleting; its node no longer " +
			"exists"
	default:
		return
	}
	if pvGone {
		f.Severity = detection.Info
	}
	f.Evidence = append(f.Evidence,
		detection.Evidence{Label: detection.EvidenceAttachVolume, Value: pv},
		detection.Evidence{Label: detection.EvidenceAttachNode, Value: node},
		detection.Evidence{Label: detection.EvidenceAttachGone, Value: gone})
	if driver != "" {
		f.Evidence = append(f.Evidence, detection.Evidence{
			Label: detection.EvidenceAttachDriver, Value: driver})
	}
}

// goneRelated is the name of the object of kind that e names through rel,
// and whether it is missing from the model. The name is empty when e
// names none.
func goneRelated(
	ctx detection.Context, e inventory.Entity, rel inventory.RelationType,
	kind inventory.Kind,
) (string, bool) {
	if ctx.Model == nil {
		return "", false
	}
	for _, id := range ctx.Model.Related(e.ID, rel, inventory.Outgoing) {
		if id.Kind == kind {
			return id.Name, ctx.Synced(kind) && !ctx.Model.Exists(id)
		}
	}
	return "", false
}
