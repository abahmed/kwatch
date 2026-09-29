package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultNamespaceTerminating is how long namespace deletion may take
// before a stuck finalizer is likely.
const DefaultNamespaceTerminating = 10 * time.Minute

// Namespace detects namespaces stuck terminating and invalid Pod Security
// labels, and LimitRanges with contradictory limits.
type Namespace struct{}

// Name implements signal.Detector.
func (Namespace) Name() string { return "namespace" }

// Kinds implements signal.Detector.
func (Namespace) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindNamespace, kube.KindLimitRange}
}

// Detect implements signal.Detector.
func (Namespace) Detect(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	if e.ID.Kind == kube.KindLimitRange {
		return invalidLimits(e)
	}
	var out []signal.Signal
	if text(e, kube.AttrPhase) == "Terminating" {
		since := valueSince(e, kube.AttrPhase)
		if sustained(ctx, since, DefaultNamespaceTerminating) {
			out = append(out, signal.Signal{
				Reason:   constant.ReasonNamespaceStuck,
				Severity: signal.Warning, Since: since,
				Summary: "Namespace has been terminating for " +
					format.Duration(ctx.Now.Sub(since)) +
					"; a finalizer is likely blocking it",
			})
		}
	}
	if problem := text(e, kube.AttrPodSecurityInvalid); problem != "" {
		out = append(out, signal.Signal{
			Reason:   constant.ReasonPodSecurityPolicyInvalid,
			Severity: signal.Warning,
			Since:    valueSince(e, kube.AttrPodSecurityInvalid),
			Summary:  "Pod Security label is invalid: " + problem,
		})
	}
	return out
}

func invalidLimits(e knowledge.Entity) []signal.Signal {
	problem := text(e, kube.AttrLimitsInvalid)
	if problem == "" {
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonLimitRangeInvalid, Severity: signal.Warning,
		Since:   valueSince(e, kube.AttrLimitsInvalid),
		Summary: "LimitRange rejects pods: " + problem,
	}}
}
