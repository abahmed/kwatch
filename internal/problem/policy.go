package problem

import (
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// digestReasons are signals worth knowing but never worth an interruption
// on their own.
var digestReasons = map[string]bool{
	constant.ReasonFailedGetResourceMetric: true,
	constant.ReasonHPAMaxedOut:             true,
	constant.ReasonTLSCertExpiringSoon:     true,
	constant.ReasonPodStuckTerminating:     true,
}

// tier derives the delivery tier from the members' severity and the
// problem's impact. A critical failure that reaches users through an
// Ingress, or a lost node, pages; a problem made only of digest signals
// waits for the digest.
func tier(p *Problem) Tier {
	worst := signal.Severity(0)
	digestOnly := true
	for key := range p.Members {
		if key.Entity == p.Root && key.Reason == constant.ReasonNodeDraining {
			// Pods disrupted by a drain or node replacement are the
			// expected cost of maintenance.
			return Digest
		}
	}
	for _, s := range p.Members {
		worst = max(worst, s.Severity)
		if !digestReasons[s.Reason] {
			digestOnly = false
		}
	}
	switch {
	case len(p.Members) == 0:
		return p.Tier
	case digestOnly || worst <= signal.Info:
		// Planned disruption or informational only.
		return Digest
	case worst == signal.Critical &&
		(userFacing(p.Impact) || p.Root.Kind == kube.KindNode):
		return Page
	default:
		return Notify
	}
}
