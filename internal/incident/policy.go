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

// Which reasons wait for the digest (detection.DigestOnly) or are causes
// without news (detection.CauseOnly) is declared with the reason, in the
// detection reason tables.

// causeOnlyReason reports a reason that is a cause but no news alone.
func causeOnlyReason(reason string) bool {
	return detection.CauseOnly(reason)
}

// causeOnly reports an incident whose members are cause-only findings
// (and advisories), with at least one of the former.
func causeOnly(p *Incident) bool {
	found := false
	for key, s := range p.Members {
		switch {
		case causeOnlyReason(key.Reason):
			found = true
		case !s.Advisory:
			return false
		}
	}
	return found
}

// advisoryState reports an incident made only of states that are not
// failures: CPU throttling, and a quota used up that nothing was refused
// by (the detector raises Warning once a create is refused). With no
// traffic lost and nothing left without a ready replica, no one is
// affected, so it produces no message of its own, not even a digest line
// (docs/production-goals.md). It stays tracked: a real failure that joins
// it makes it speak.
func advisoryState(p *Incident) bool {
	if p.trafficLost || p.servingDown || len(p.Members) == 0 {
		return false
	}
	found := false
	for key, s := range p.Members {
		switch {
		case key.Reason == reasons.ContainerCPUThrottled,
			key.Reason == reasons.ResourceQuotaExhausted &&
				s.Severity <= detection.Info:
			found = true
		case !s.Advisory:
			return false
		}
	}
	return found
}

// digestReason reports whether a finding reason waits for the digest.
// Repeated unknown Warning events are worth a look, not an interruption,
// and so is a node condition kwatch has no detector for that has lasted:
// it is one digest line, and the cause of the pods that fail on the node.
func digestReason(reason string) bool {
	return detection.DigestOnly(reason) ||
		strings.HasPrefix(reason, reasons.UnusualEventPrefix) ||
		strings.HasPrefix(reason, reasons.NodeConditionPrefix) ||
		strings.HasPrefix(reason, reasons.RiskPrefix)
}

// waitsForDigest reports whether a finding waits for the digest. An
// expired certificate does so unless it is critical: the detector marks
// it critical only when something uses it, and one nothing references
// breaks nothing.
func waitsForDigest(s detection.Finding) bool {
	if s.Reason == reasons.TLSCertExpired {
		return s.Severity < detection.Critical
	}
	return digestReason(s.Reason)
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
	t, _ := tierWhy(p)
	return t
}

// tierWhy is tier with the code of why (see level.go). A fixed tier of
// an incident without members has no code: the earlier one stands.
func tierWhy(p *Incident) (Tier, string) {
	if fixed, ok := fixedTier(p); ok {
		return fixed, fixedLevel(p)
	}
	if quiet, why, ok := quietTier(p); ok {
		return quiet, why
	}
	worst, digestOnly := interrupting(p)
	if digestOnly || worst <= detection.Info {
		// Planned disruption or informational only.
		return Digest, levelDigestOnly
	}
	if worst == detection.Critical || p.criticalRoot() {
		if rule := pageRuleOf(p); rule != "" {
			if p.Delivery.PageHeld() {
				// A repeat of a page that just resolved: no new page.
				return Notify, levelPageHeld
			}
			return Page, levelPagePrefix + rule
		}
	}
	if chronicLow(p, worst) {
		return Digest, chronicLevel(p)
	}
	return Notify, levelNotify
}

// quietTier is the tier of an incident nobody needs to hear of now,
// with its code: a cause or a state that affects no one is silent, and
// a failure people know well waits for the digest.
func quietTier(p *Incident) (Tier, string, bool) {
	switch {
	case causeOnly(p):
		return Silent, levelCauseOnly, true
	case advisoryState(p):
		return Silent, levelAdvisory, true
	case routine(p) && !p.persistent && !unusual(p):
		// Happens at the same time every day and resolves on its own:
		// learned normal, reported in the digest. One that outlasts
		// the boot window with a crash loop or nothing ready does not
		// resolve on its own (see escalate). An outage a page rule
		// names (a node lost, traffic down) is never only a digest
		// line, however regular: it still notifies.
		if pageRuleOf(p) != "" {
			return Notify, levelRoutine, true
		}
		return Digest, levelRoutine, true
	case reachedPaging(p) || p.persistent || unusual(p):
		return 0, "", false
	case Known(*p, p.Opened):
		// Heard about for a day, or failing on a regular rhythm: not
		// news any more. The digest keeps counting it, unless it
		// crash-loops or has nothing ready past the boot window:
		// that is not routine boot noise (see escalate).
		return Digest, levelKnown, true
	case hasRhythm(p):
		return Digest, levelRhythm, true
	}
	return 0, "", false
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
		if s.Advisory || waitsForDigest(s) ||
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
		if !detection.DigestOnly(s.Reason) && !waitsForDigest(s) {
			return Notify
		}
		tier = Digest
	}
	return tier
}

// routine reports an incident that opened within routineWindow of the
// same time of day on at least routineDays of the recorded days, and
// went away on its own those days: a day someone fixed it (a rollback,
// a config change, History) is no routine, it is a repeated outage.
func routine(p *Incident) bool {
	if len(p.Occurrences) < routineDays {
		return false
	}
	fixed := map[string]bool{}
	for _, o := range p.History {
		if o.Fix != "" && o.Fix != FixNone {
			fixed[dayOf(o.Opened)] = true
		}
	}
	latest := p.Occurrences[len(p.Occurrences)-1]
	days := map[string]bool{}
	for _, at := range p.Occurrences {
		if timeOfDayDistance(at, latest) <= routineWindow &&
			!fixed[dayOf(at)] {
			days[dayOf(at)] = true
		}
	}
	return len(days) >= routineDays
}

func dayOf(at time.Time) string { return at.UTC().Format("2006-01-02") }

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
