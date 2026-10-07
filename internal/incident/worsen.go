package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Escalation: news that is not a new object failing but the failure
// getting worse, or lasting longer than the policy tolerates.

// stage ranks how badly an incident's members fail. The fingerprint reads
// the peak, so each step up is one update and a dip is never news.
type stage uint8

const (
	// stageFailing is any failing member: not ready, pending, unavailable.
	stageFailing stage = iota + 1
	// stageCrashLoop is a member that crash-loops or is killed over and
	// over: the workload does not merely wait, it fails.
	stageCrashLoop
)

// crashLooping reports a finding of a container that keeps failing.
func crashLooping(s detection.Finding) bool {
	return s.Mode == detection.ModeCrashLoop ||
		s.Mode == detection.ModeOOMKilled
}

// memberStage is the worst stage among the failing members of p.
func memberStage(p *Incident) stage {
	worst := stage(0)
	for _, s := range p.Members {
		if s.Advisory {
			continue
		}
		worst = max(worst, stageFailing)
		if crashLooping(s) {
			return stageCrashLoop
		}
	}
	return worst
}

// noteStage raises the stage peak the fingerprint reads.
func (p *Incident) noteStage() {
	p.stagePeak = max(p.stagePeak, memberStage(p))
}

// lastingCrash reports a Critical crash-looping member.
func lastingCrash(p *Incident) bool {
	for _, s := range p.Members {
		if !s.Advisory && s.Severity == detection.Critical &&
			crashLooping(s) {
			return true
		}
	}
	return false
}

// maxedSince is the earliest Since among the HPAMaxedOut members, and
// whether there is one.
func maxedSince(p *Incident) (time.Time, bool) {
	var first time.Time
	found := false
	for key, s := range p.Members {
		if key.Reason != reasons.HPAMaxedOut || s.Since.IsZero() {
			continue
		}
		if !found || s.Since.Before(first) {
			first, found = s.Since, true
		}
	}
	return first, found
}

// escalate re-judges the tier of a digest-tier incident once time has
// made a habit-demoted failure lasting, or an autoscaler's maximum
// stuck. It reports whether the tier changed.
//
// Known and rhythmic incidents are demoted to the digest because their
// boot noise is routine. A member that crash-loops past the boot window,
// or a workload with nothing ready past it, is not boot noise.
func (m *Manager) escalate(p *Incident, now time.Time) bool {
	if p.Tier != Digest {
		return false
	}
	if due, ok := escalationDeadline(p); !ok || now.Before(due) {
		return false
	}
	if !p.persistent && m.lastsPastBoot(p, now) {
		p.persistent = true
	}
	if since, ok := maxedSince(p); ok && !p.maxedLong &&
		now.Sub(since) >= HPAStuckAfter {
		p.maxedLong = true
	}
	next := m.override.apply(p, tier(p))
	if next <= p.Tier {
		return false
	}
	p.Tier = next
	return true
}

// lastsPastBoot reports a failure that is not boot noise any more: the
// incident has lasted the boot window and a member keeps crashing or
// the workload has nothing ready. escalate and the coverage check both
// read it, so they agree on when a quiet incident must speak.
func (m *Manager) lastsPastBoot(p *Incident, now time.Time) bool {
	start := p.bootStart()
	return !start.IsZero() && now.Sub(start) >= kube.BootWindow &&
		(lastingCrash(p) || m.workloadDown(p))
}

// bootStart is when the boot window of p began: when it opened, or, for
// an incident restored after a gap, when this run started watching. The
// pods of a restored incident boot again after the restart, and their
// crashes are boot noise measured from that boot, not from last night.
// A workload whose pods have not been ready since before that moment is
// the exception: the kubelet's own time says it did not boot with this
// run, so its window began then.
func (p *Incident) bootStart() time.Time {
	start := p.Opened
	if p.bootedAt.After(start) {
		start = p.bootedAt
	}
	if since := p.unreadySince(); !since.IsZero() && since.Before(start) {
		return since
	}
	return start
}

// unreadySince is the earliest time the kubelet recorded for a
// WorkloadNeverReady member that reports a failure (not a risk), or zero.
func (p *Incident) unreadySince() time.Time {
	var first time.Time
	for key, s := range p.Members {
		if key.Reason != reasons.WorkloadNeverReady || s.Advisory ||
			s.Since.IsZero() {
			continue
		}
		if first.IsZero() || s.Since.Before(first) {
			first = s.Since
		}
	}
	return first
}

// workloadDown reports that a workload the incident speaks for has no
// ready replica: the one at its root, or the owner of any member. A
// failure explained by another root (a node pool, a node) is still the
// workload's outage, and the incident of that root must not stay quiet
// about it because its own problem is known.
func (m *Manager) workloadDown(p *Incident) bool {
	if m.model == nil {
		return false
	}
	if workloadHasNoneReady(m.model, p.Root) {
		return true
	}
	for key, s := range p.Members {
		if !s.Advisory && workloadHasNoneReady(m.model, key.Entity) {
			return true
		}
	}
	return false
}

// workloadHasNoneReady reports that the workload owning id wants
// replicas and has none ready. Anything that is not, or has no, workload
// is not down.
func workloadHasNoneReady(
	model inventory.Reader, id inventory.EntityID,
) bool {
	r, ok := ReadinessOf(model, workloadFor(model, id))
	return ok && r.Desired > 0 && r.Ready == 0
}

// escalationDeadline is when an announced digest-tier incident next
// deserves a second look at its tier.
func escalationDeadline(p *Incident) (time.Time, bool) {
	if p.Tier != Digest || p.bootStart().IsZero() {
		return time.Time{}, false
	}
	var due time.Time
	ok := false
	consider := func(at time.Time) {
		if !ok || at.Before(due) {
			due, ok = at, true
		}
	}
	if !p.persistent {
		consider(p.bootStart().Add(kube.BootWindow))
	}
	if since, found := maxedSince(p); found && !p.maxedLong {
		consider(since.Add(HPAStuckAfter))
	}
	return due, ok
}

// materialHeld reports a material-change update that must wait: the
// last one went out less than MaterialGap ago and the tier did not rise.
func materialHeld(p *Incident, now time.Time) bool {
	last := p.Delivery.LastMaterial()
	return !last.IsZero() && now.Before(last.Add(MaterialGap)) &&
		p.Tier <= p.Delivery.SentTier()
}
