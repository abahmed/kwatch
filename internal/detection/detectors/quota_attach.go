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

// DefaultDetachStuck is how long a VolumeAttachment may take to detach;
// the attach-detach controller force-detaches after six minutes.
const DefaultDetachStuck = 10 * time.Minute

// detachFailure reports a volume that cannot be detached: the CSI driver
// returned a detach error, or deletion of the attachment has been pending
// for DefaultDetachStuck. Until it detaches, the volume cannot attach to
// another node.
func detachFailure(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	message := text(e, kube.AttrDetachError)
	deleting := flag(e, kube.AttrDeleting)
	if message == "" && !deleting {
		return nil
	}
	since := valueSince(e, kube.AttrDetachError)
	summary := "Volume cannot be detached from its node"
	var evidence []detection.Evidence
	if message != "" {
		evidence = append(evidence,
			detection.Evidence{Label: "detach error", Value: message})
	} else {
		since = timestamp(e, kube.AttrDeletingSince)
		if since.IsZero() {
			since = valueSince(e, kube.AttrDeleting)
		}
		if !sustained(ctx, "detach-stuck", since, DefaultDetachStuck) {
			return nil
		}
		summary = "Volume has been detaching for " +
			format.Duration(ctx.Now.Sub(since))
	}
	return []detection.Finding{{
		Reason: reasons.VolumeDetachFailure, Severity: detection.Warning,
		Health: detection.Failing, Since: since, Summary: summary,
		Evidence: evidence,
	}}
}
