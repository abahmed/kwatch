package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// Delivery policy. These values decide who is interrupted and when; they
// are product decisions, kept together so they can be read and changed in
// one place. tier applies them.

// digestReasons are findings worth knowing but never worth an
// interruption on their own: an incident made only of them waits for the
// digest.
var digestReasons = map[string]bool{
	reasons.FailedGetResourceMetric: true,
	reasons.HPAMaxedOut:             true,
	reasons.TLSCertExpiringSoon:     true,
	reasons.PodStuckTerminating:     true,
	reasons.ContainerCPUHigh:        true,
	reasons.ContainerCPUThrottled:   true,
	reasons.NodeResourceHigh:        true,
}

// A routine incident happens at about the same time of day, again and
// again, and resolves on its own: a nightly batch job, a daily restart.
// It is learned normal and goes to the digest.
const (
	// routineWindow is how close to the same time of day occurrences
	// must be.
	routineWindow = 45 * time.Minute
	// routineDays is how many distinct days, the current one included,
	// must have such an occurrence.
	routineDays = 3
)

// tier derives the delivery tier from the members' severity and the
// incident's impact. A critical incident that matches one of pageRules
// pages; an incident made only of digest findings waits for the digest.
func tier(p *Incident) Tier {
	worst := detection.Severity(0)
	digestOnly := true
	for _, s := range p.Members {
		worst = max(worst, s.Severity)
		if !digestReasons[s.Reason] {
			digestOnly = false
		}
	}
	switch {
	case len(p.Members) == 0:
		return p.Tier
	case drainingRoot(p):
		return drainTier(p)
	case routine(p):
		// Happens at the same time every day and resolves on its own:
		// learned normal, reported in the digest.
		return Digest
	case digestOnly || worst <= detection.Info:
		// Planned disruption or informational only.
		return Digest
	case (worst == detection.Critical || p.criticalRoot()) &&
		pageRuleOf(p) != "":
		return Page
	default:
		return Notify
	}
}

// drainingRoot reports an incident rooted at a node that is cordoned or
// being removed: a drain, an upgrade, a scale-down or a spot
// replacement.
func drainingRoot(p *Incident) bool {
	for key := range p.Members {
		if key.Entity == p.Root && key.Reason == reasons.NodeDraining {
			return true
		}
	}
	return false
}

// drainTier tiers a drain by whether it stayed inside its envelope.
// The node's own findings (cordoned, NotReady while it shuts down) are
// the maintenance itself. A drain within its envelope disrupts nothing
// for long: it is silent, with no digest and no recovery message. It
// exceeds the envelope when something else fails because of it: pods
// that stay unready long after, a budget that blocks evictions. That
// notifies, or reaches the digest when those failures are only digest
// findings. A drain never pages: the node was taken out on purpose.
func drainTier(p *Incident) Tier {
	tier := Silent
	for _, s := range p.Members {
		if s.Entity == p.Root || s.Severity <= detection.Info {
			continue
		}
		if !digestReasons[s.Reason] {
			return Notify
		}
		tier = Digest
	}
	return tier
}

// routine reports an incident that opened within routineWindow of the
// same time of day on at least routineDays of the recorded days.
func routine(p *Incident) bool {
	if len(p.Occurrences) < routineDays {
		return false
	}
	latest := p.Occurrences[len(p.Occurrences)-1]
	days := map[string]bool{}
	for _, at := range p.Occurrences {
		if timeOfDayDistance(at, latest) <= routineWindow {
			days[at.UTC().Format("2006-01-02")] = true
		}
	}
	return len(days) >= routineDays
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
