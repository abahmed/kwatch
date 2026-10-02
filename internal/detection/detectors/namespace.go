package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultNamespaceTerminating is how long namespace deletion may take
// before a stuck finalizer is likely.
const DefaultNamespaceTerminating = 10 * time.Minute

// Namespace detects namespaces stuck terminating and invalid Pod Security
// labels, and LimitRanges with contradictory limits.
type Namespace struct{}

// Name implements detection.Detector.
func (Namespace) Name() string { return "namespace" }

// Kinds implements detection.Detector.
func (Namespace) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindNamespace, kube.KindLimitRange}
}

// Detect implements detection.Detector.
func (Namespace) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if e.ID.Kind == kube.KindLimitRange {
		return invalidLimits(e)
	}
	var out []detection.Finding
	if text(e, kube.AttrPhase) == "Terminating" {
		since := valueSince(e, kube.AttrPhase)
		if sustained(ctx, "namespace-terminating", since,
			DefaultNamespaceTerminating) {
			out = append(out, detection.Finding{
				Reason:   reasons.NamespaceStuck,
				Severity: detection.Warning, Since: since,
				Summary: "Namespace has been terminating for " +
					format.Duration(ctx.Now.Sub(since)) +
					"; a finalizer is likely blocking it",
			})
		}
	}
	if incident := text(e, kube.AttrPodSecurityInvalid); incident != "" {
		out = append(out, detection.Finding{
			Reason:   reasons.PodSecurityPolicyInvalid,
			Severity: detection.Warning,
			Since:    valueSince(e, kube.AttrPodSecurityInvalid),
			Summary:  "Pod Security label is invalid: " + incident,
		})
	}
	return out
}

func invalidLimits(e inventory.Entity) []detection.Finding {
	incident := text(e, kube.AttrLimitsInvalid)
	if incident == "" {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.LimitRangeInvalid, Severity: detection.Warning,
		Since:   valueSince(e, kube.AttrLimitsInvalid),
		Summary: "LimitRange rejects pods: " + incident,
	}}
}
