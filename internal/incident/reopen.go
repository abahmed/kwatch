package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// ReasonFailingAgain is the audit reason of the update that tells an
// announced incident re-opened within RepageWindow of its resolve.
const ReasonFailingAgain Reason = "failing again"

// reopenable reports whether the resolved incident p is the announced
// incident that a failure of the same root and mode just continued: it
// notified or paged, it was told about, it ended on its own less than
// RepageWindow ago. Such an outage is flapping, so it comes back as the
// same incident, and not as a new one with a new thread and, for a
// page, a new paging alert. A finding with no
// mode, or an incident without one, matches any mode.
func reopenable(
	p *Incident, now time.Time, mode detection.Mode,
) bool {
	return p.State == Resolved && p.CanReopen() &&
		now.Sub(p.Resolved) <= RepageWindow &&
		p.hasMode(mode)
}

// CanReopen reports whether a failure within RepageWindow of this
// incident's resolve would continue it: people were told about it (an
// announcement that was only decided, held and dropped, or out of scope
// does not count), it notified, paged or was listed by a digest, and it
// was not superseded. The resolve message carries
// RepageWindow when this holds, so chat threads stay open for the reopen.
func (p Incident) CanReopen() bool {
	return p.SupersededBy == "" && p.sent.told &&
		(p.Tier >= Notify || p.Tier == Digest || p.Delivery.PageHeld())
}

// reopen makes the resolved incident p live again under the same ID and
// alert key. A page notifies instead of paging again and keeps its
// reminders; a notification stays one. One update, "failing again", is
// sent once the members settled (see reopenUpdate). The flap bookkeeping
// is the same as for a recurrence: cycles and occurrences carry on, so
// the hold still grows.
func (m *Manager) reopen(p *Incident, now time.Time) {
	m.dropResolved(p.ID, p.Root.String())
	p.History = appendHistory(p.History, occurrenceOf(p))
	p.Cycles = recent(p.Cycles, now, m.cfg.FlapWindow)
	if now.Sub(p.Resolved) <= m.cfg.FlapWindow {
		p.Cycles = append(p.Cycles, now)
	}
	p.Occurrences = appendOccurrence(p.Occurrences, now)
	times := heardTimes(p, now)
	if isPage(p) {
		times, _ = repeatsPage(p, now)
		p.Delivery.HoldAtNotify()
	}
	p.State, p.Opened, p.Announced = Open, now, now
	p.Resolved, p.RecoveringSince, p.Reminded = time.Time{}, time.Time{},
		time.Time{}
	p.Fix, p.FixedBy = "", nil
	// A fix attempt of the earlier occurrence says nothing about this one.
	p.Attempt, p.attemptSeen, p.attemptLate = nil, nil, false
	if p.Tier != Digest {
		// A digest incident stays one: its return is a line in the next
		// digest, not an interruption.
		p.Tier = Notify
	}
	p.Pending.ScheduleReopenUpdate(now)
	p.RepeatCount = times
	p.note(now, repeatNote(times))
	if len(p.Cycles) >= m.cfg.FlapCycles {
		p.State = Flapping
	}
	m.changed[p.ID] = true
}

// reopenUpdate sends the "failing again" update of a reopened incident
// once its members had ReviseSettle to arrive, so one message carries
// them all; until then the incident says nothing else. It is sent even
// while the incident flaps: the thread must say the outage is back.
func (m *Manager) reopenUpdate(p *Incident, now time.Time) (Decision, bool) {
	if !p.Pending.DueReopenUpdate(now, m.cfg.ReviseSettle) {
		return Decision{}, false
	}
	p.Pending.ClearReopen()
	return m.decide(p, Update, ReasonFailingAgain), true
}

// reopenPending reports a reopened incident whose update is still due.
// A reopen that recovered first keeps it: the thread's last word is the
// old resolve, so the update goes out when the failure returns. It waits
// until the incident is Open again, so the update describes an open
// incident and the next tick sees no change.
func (m *Manager) reopenPending(p *Incident) bool {
	return p.Pending.ReopenOwed() && len(p.Members) > 0 &&
		wasAnnounced(p) && p.State != Recovering
}

// incident returns the live incident for root, opening a new one when
// there is none.
func (m *Manager) incident(
	now time.Time, root inventory.EntityID, mode detection.Mode,
) *Incident {
	previous := m.lookup(root)
	if previous != nil && previous.State != Resolved {
		return previous
	}
	if previous != nil && reopenable(previous, now, mode) {
		m.reopen(previous, now)
		return previous
	}
	p := &Incident{
		ID: m.newID(now), Root: root, State: Settling, Opened: now,
		Members:     make(map[detection.Key]detection.Finding),
		Occurrences: []time.Time{now},
	}
	if previous != nil {
		m.recur(p, previous, now)
	}
	m.incidents[p.ID] = p
	m.index(p)
	m.noteOpened(p.ID)
	return p
}

// recur links a new incident to the resolved incident of the same root
// and carries its history over, so flapping and daily routines are still
// recognised. A recurrence within FlapWindow of the resolve counts as a
// cycle, so an incident that stays healthy longer than its hold between
// failures still doubles its hold and flaps.
func (m *Manager) recur(p, previous *Incident, now time.Time) {
	p.Previous = previous.ID
	p.Cycles = recent(previous.Cycles, now, m.cfg.FlapWindow)
	if now.Sub(previous.Resolved) <= m.cfg.FlapWindow {
		p.Cycles = append(p.Cycles, now)
	}
	p.Occurrences = appendOccurrence(previous.Occurrences, now)
	if previous.SupersededBy == "" {
		p.History = appendHistory(previous.History, occurrenceOf(previous))
	}
	week := recent(p.Occurrences, now, recurrenceWeek)
	p.note(now, "happened again ("+ordinal(len(week))+
		" time this week, last time "+previous.ID+")")
	// The recurrence carries everything the predecessor knew, so keeping
	// the predecessor would only grow memory with every blip.
	m.forget(previous)
}
