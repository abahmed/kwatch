package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/format"
)

// Timers are what a live incident waited on at the latest tick: the
// settle before its first message, the healthy time it needs before it
// closes and its next timer. They are computed with the deadlines and
// never decide anything themselves.
type Timers struct {
	// At is the tick that computed them; zero before the first.
	At time.Time
	// Settle is the wait from Opened before the first message, for a
	// settling incident.
	Settle time.Duration
	// Hold is how long the incident must stay healthy (stable, when it
	// flaps) before it closes.
	Hold time.Duration
	// Next is the incident's next timer; zero when none is pending.
	Next time.Time
	// Grace marks a restored incident waiting for the cluster to be
	// read again before it judges recovery.
	Grace bool
	// Held marks an incident with no failure left that stays open
	// because kwatch cannot observe its root now (see awaitingEvidence):
	// no hold runs until it can check again.
	Held bool
}

// noteTimers stores p's timers as of now. next is its deadline when ok;
// burst says enough pages settle at once that a page waits the full
// settle (see settleFor).
func (m *Manager) noteTimers(
	p *Incident, now, next time.Time, ok, burst bool,
) {
	t := Timers{At: now, Grace: m.inGrace(p, now),
		Held: p.heldForData && len(p.Members) == 0}
	if ok {
		t.Next = next
	}
	switch p.State {
	case Settling:
		t.Settle = m.cfg.Settle
		if p.Tier == Page && !burst {
			t.Settle = m.cfg.PageSettle
		}
	case Flapping:
		t.Hold = m.cfg.MaxHold
	default:
		t.Hold = m.holdFor(p, now)
	}
	p.Journal.Timers = t
}

// Why says why an incident is at its alert level and why it is still
// open, with the hold it waits out and its next check.
type Why struct {
	AlertLevel  string
	StillOpen   string
	HoldTotal   time.Duration
	HoldElapsed time.Duration
	NextCheckAt time.Time
	// CannotSee says kwatch cannot observe the root right now, so the
	// incident stays open with no hold running until it can check again.
	CannotSee bool
}

// WhyOf explains p as of now from its journal: the level code and the
// timers of its latest tick. A resolved incident is not open, so only
// its level is said.
func WhyOf(p Incident, now time.Time) Why {
	t := p.Journal.Timers
	w := Why{AlertLevel: AlertLevelText(p.Journal.Level)}
	if p.State == Resolved {
		return w
	}
	if t.Next.After(now) {
		w.NextCheckAt = t.Next
	}
	w.CannotSee = t.Held && !t.Grace
	if t.Hold > 0 && p.State != Settling && !w.CannotSee {
		w.HoldTotal = t.Hold
		if p.State != Open && !p.RecoveringSince.IsZero() &&
			len(p.Members) == 0 {
			w.HoldElapsed = min(max(now.Sub(p.RecoveringSince), 0), t.Hold)
		}
	}
	w.StillOpen = stillOpen(p, t, now, w)
	return w
}

// stillOpen says in plain words why p has not closed.
func stillOpen(p Incident, t Timers, now time.Time, w Why) string {
	switch {
	case t.Grace:
		return "kwatch just restarted, so it is double-checking the " +
			"cluster before deciding whether it recovered"
	case w.CannotSee:
		return "kwatch can't see " + describe(p.Root) +
			" right now, so it stays open until it can check again"
	case p.State == Settling:
		return settlingText(p, t, now)
	case p.State == Flapping && len(p.Members) > 0:
		return "Flapping: failing again" + closesAfter(t.Hold, " stable")
	case p.State == Flapping:
		return "Flapping: waiting for " + format.Duration(t.Hold) +
			" stable, " + format.Duration(w.HoldElapsed) + " so far"
	case p.State == Recovering && w.HoldElapsed >= t.Hold && t.Hold > 0:
		return "Looks better: healthy for " + format.Duration(t.Hold) +
			"; closes once its workload is fully ready again"
	case p.State == Recovering:
		return "Looks better; closes after " + format.Duration(t.Hold) +
			" healthy, " + format.Duration(w.HoldElapsed) + " so far"
	}
	return "Happening now" + closesAfter(t.Hold, " healthy")
}

// closesAfter is "; closes after 3m healthy", or "" without a hold.
func closesAfter(hold time.Duration, what string) string {
	if hold <= 0 {
		return ""
	}
	return "; closes after " + format.Duration(hold) + what
}

// settlingText says why a settling incident has not alerted yet.
func settlingText(p Incident, t Timers, now time.Time) string {
	switch {
	case p.Tier == Silent:
		return "Not alerting: nothing in it is worth a message"
	case len(p.Members) == 0:
		return "Confirming: its failures cleared, so it closes " +
			"without sending a message"
	case t.Settle <= 0:
		return "Confirming: making sure it still fails before alerting"
	}
	left := p.Opened.Add(t.Settle).Sub(now)
	if left <= 0 {
		return "Confirming: alerts at the next check"
	}
	return "Confirming: making sure it still fails before alerting, " +
		format.Duration(left) + " left"
}
