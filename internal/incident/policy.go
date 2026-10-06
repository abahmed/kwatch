package incident

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
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
	// A node whose workloads stall on CPU, memory or disk is under
	// strain, not failing: when a pod on it fails, that pod is the news.
	reasons.NodePSIHigh: true,
	// A budget selecting no pods protects nothing, but on a cluster that
	// scales workloads to zero it is the normal night.
	reasons.PdbSelectsNothing: true,
	// An overcommitted node is a risk until something is killed; the
	// kill is the incident, and this is its cause.
	reasons.NodeMemoryOvercommitted: true,
	// Evictions are the kubelet doing its job; the evicted pods are
	// the incidents. Idle objects are clutter, not failures.
	reasons.NodeEvicting:  true,
	reasons.ServiceUnused: true,
	reasons.ClaimUnused:   true,
	// A scale target that does not exist and a Ready node whose kubelet
	// kwatch cannot reach are worth a look; whatever fails because of
	// them is the incident.
	reasons.HPATargetMissing:   true,
	reasons.KubeletUnreachable: true,
}

// digestReason reports whether a finding reason waits for the digest.
// Repeated unknown Warning events are worth a look, not an interruption.
func digestReason(reason string) bool {
	return digestReasons[reason] ||
		strings.HasPrefix(reason, reasons.UnusualEventPrefix) ||
		strings.HasPrefix(reason, reasons.RiskPrefix)
}

// A routine incident happens at about the same time of day, again and
// again, and resolves on its own: a nightly batch job, a daily restart.
// It is learned normal and goes to the digest.
const (
	// routineDays is how many distinct days, the current one included,
	// must have such an occurrence.
	routineDays = 3
)

// tier derives the delivery tier from the members' severity and the
// incident's impact. A critical incident that matches one of pageRules
// pages; an incident made only of digest findings waits for the digest.
func tier(p *Incident) Tier {
	worst, digestOnly := interrupting(p)
	switch {
	case len(p.Members) == 0:
		return p.Tier
	case drainingRoot(p):
		return drainTier(p)
	case routine(p) && !p.persistent && !unusual(p):
		// Happens at the same time every day and resolves on its own:
		// learned normal, reported in the digest. One that outlasts
		// the boot window with a crash loop or nothing ready does not
		// resolve on its own (see escalate).
		return Digest
	case digestOnly && p.maxedLong:
		// An autoscaler at its maximum for a long time has no headroom
		// left; one notification says so (the tier never falls back).
		return Notify
	case !reachedPaging(p) && !p.persistent && !unusual(p) &&
		(Known(*p, p.Opened) || hasRhythm(p)):
		// Heard about for a day, or failing on a regular rhythm: not
		// news any more. The digest keeps counting it, unless it
		// crash-loops or has nothing ready past the boot window:
		// that is not routine boot noise (see escalate).
		return Digest
	case digestOnly || worst <= detection.Info:
		// Planned disruption or informational only.
		return Digest
	case (worst == detection.Critical || p.criticalRoot()) &&
		pageRuleOf(p) != "":
		if p.Delivery.PageHeld() {
			// A repeat of a page that just resolved: no new page.
			return Notify
		}
		return Page
	default:
		return Notify
	}
}

// interrupting is the worst severity among the members that may
// interrupt anyone, and whether there is none. A risk, a digest finding
// or behaviour that is usual for the workload never raises the tier, at
// any severity: it waits for the digest. Usual stops excusing once the
// workload has been down past the boot window (see escalate): history
// never silences an outage.
func interrupting(p *Incident) (worst detection.Severity, digestOnly bool) {
	digestOnly = true
	for _, s := range p.Members {
		if s.Advisory || digestReason(s.Reason) ||
			(s.Normal == detection.NormalUsual && !p.persistent) {
			continue
		}
		worst = max(worst, s.Severity)
		digestOnly = false
	}
	return worst, digestOnly
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
	day := format.Day
	da := a.UTC().Sub(a.UTC().Truncate(day))
	db := b.UTC().Sub(b.UTC().Truncate(day))
	diff := da - db
	if diff < 0 {
		diff = -diff
	}
	return min(diff, day-diff)
}

// hasRhythm reports whether the incident's occurrences show a rhythm.
func hasRhythm(p *Incident) bool {
	_, ok := Rhythm(*p, p.Opened)
	return ok
}
