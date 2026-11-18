package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Old problems already in an incident. agesOut keeps an old low
// finding out of the incidents when it first raises, but an incident
// opened before the rule existed, or restored from the state file,
// never met it. So every Apply looks at the live incidents again: one
// whose every failure is an old low problem by the same rule (low on
// its own, already older than the horizon when the incident opened,
// not part of a group, still active) is closed without a message, and
// its failures join the old problems. A change brings them back the
// usual way: agesOut lets a finding that rose (rose) open again, and
// noteRose keeps it from being lowered for its age.

// ReasonOldProblem closes an incident whose every failure is an old
// problem: it sends no message, so it is not in AllReasons.
const ReasonOldProblem Reason = "listed under old problems"

// retireOldProblems closes, quietly, every live incident made only of
// old problems, and lists those under the old problems.
func (m *Manager) retireOldProblems(s explain.Snapshot) {
	if m.cfg.OldProblem <= 0 {
		return
	}
	for _, p := range m.incidents {
		if old := m.oldOnly(p, s); len(old) > 0 {
			m.retireOld(p, old, s.Now)
		}
	}
}

// oldOnly returns the active findings of p's failures when every one is
// an old problem; nil otherwise. An incident that rose, that holds an
// alert at a pager or that is closed already is left alone.
func (m *Manager) oldOnly(
	p *Incident, s explain.Snapshot,
) []detection.Finding {
	if p.State == Resolved || p.marks.Rose || p.Opened.IsZero() ||
		p.Delivery.OpenAtPagers() {
		return nil
	}
	where := placement{root: p.Root, cause: p.Cause}
	var out []detection.Finding
	for key, was := range p.Members {
		if was.Advisory {
			continue
		}
		f, ok := activeFinding(s, key)
		if !ok || !m.oldWhenOpened(p, f, where) {
			return nil
		}
		out = append(out, f)
	}
	return out
}

// oldWhenOpened reports a low finding of p whose onset was already
// past the horizon when p opened, and that no group holds: the rule of
// agesOut, measured when kwatch first saw it.
func (m *Manager) oldWhenOpened(
	p *Incident, f detection.Finding, where placement,
) bool {
	return lowAlone(f) && !f.Since.IsZero() &&
		p.Opened.Sub(f.Since) > m.cfg.OldProblem && !m.inGroup(f, where)
}

// activeFinding is the finding of key the snapshot holds now.
func activeFinding(
	s explain.Snapshot, key detection.Key,
) (detection.Finding, bool) {
	for _, f := range s.Findings[key.Entity] {
		if f.Key() == key {
			return f, true
		}
	}
	return detection.Finding{}, false
}

// retireOld closes p without a message and lists its failures, old,
// with the old problems. Its configuration risks leave with it.
func (m *Manager) retireOld(
	p *Incident, old []detection.Finding, now time.Time,
) {
	for key := range p.Members {
		delete(m.byMember, key)
	}
	clear(p.Members)
	if m.aged == nil {
		m.aged = map[detection.Key]detection.Finding{}
	}
	for _, f := range old {
		m.aged[f.Key()] = f
	}
	p.State, p.Resolved, p.quiet = Resolved, now, true
	p.Held = false
	p.Pending.ClearReopen()
	p.Pending.ClearRevised()
	m.quietIDs = append(m.quietIDs, p.ID)
	p.note(now, "resolved: "+string(ReasonOldProblem))
	m.keepLevel(p)
	logResolved(p, now, ReasonOldProblem, true)
	m.markResolved(p)
}

// liveEntities are the objects a live incident is about: its root and
// the object of each member. An old problem of one of them is not
// listed apart: the object is already on the issues list.
func (m *Manager) liveEntities() map[inventory.EntityID]bool {
	out := map[inventory.EntityID]bool{}
	for _, p := range m.incidents {
		if p.State == Resolved {
			continue
		}
		out[p.Root] = true
		for key := range p.Members {
			out[key.Entity] = true
		}
	}
	return out
}
