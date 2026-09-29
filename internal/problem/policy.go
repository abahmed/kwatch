package problem

import (
	"time"

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
	constant.ReasonContainerCPUHigh:        true,
	constant.ReasonContainerCPUThrottled:   true,
	constant.ReasonNodeResourceHigh:        true,
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
	case routine(p):
		// Happens at the same time every day and resolves on its own:
		// learned normal, reported in the digest.
		return Digest
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

// routineWindow is how close to the same time of day occurrences must be.
const routineWindow = 45 * time.Minute

// routine reports a problem that opened at about the same time of day on
// at least three of the recorded days, the current one included.
func routine(p *Problem) bool {
	if len(p.Occurrences) < 3 {
		return false
	}
	latest := p.Occurrences[len(p.Occurrences)-1]
	days := map[string]bool{}
	for _, at := range p.Occurrences {
		if timeOfDayDistance(at, latest) <= routineWindow {
			days[at.UTC().Format("2006-01-02")] = true
		}
	}
	return len(days) >= 3
}

func timeOfDayDistance(a, b time.Time) time.Duration {
	day := 24 * time.Hour
	da := a.UTC().Sub(a.UTC().Truncate(day))
	db := b.UTC().Sub(b.UTC().Truncate(day))
	diff := da - db
	if diff < 0 {
		diff = -diff
	}
	return min(diff, day-diff)
}
