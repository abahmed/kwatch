package incident

import "time"

// A step looks at one open incident and either settles the tick (done)
// or leaves it to the next step. A done step with a zero Decision says
// nothing this tick.
type step func(m *Manager, p *Incident, now time.Time) (d Decision, done bool)

// openSteps is the order in which an open incident's news is considered.
// The order matters: earlier steps hold the later ones back so that one
// tick sends at most one update, and no update swallows news.
//
//  1. evidence: no members means recovery, or waiting for evidence.
//  2. revised: a new cause goes first, after ReviseSettle collects what
//     joins it.
//  3. attempt: a fix attempt, only when no material change is pending.
//  4. ack: an acknowledgement that appeared or went is told once.
//  5. reminder: unchanged content may be said again when it is due.
//  6. material: the content changed (a tier rise, more failing, a worse
//     stage), spaced by MaterialGap.
//
// A reopened incident's "failing again" update comes before all of these,
// in advance.
//
// Updates come from four places, not only this list:
//  1. openSteps (this file): news of an Open incident.
//  2. reopenUpdate (reopen.go, called from advance): "failing again",
//     before the steps.
//  3. recovering and flapping handlers (lifecycle.go): the "flapping"
//     update when recoveries reach FlapCycles, and what a flapping
//     incident says (flapping, flappingNews).
//  4. escalate (worsen.go, called from advance): raises the tier without
//     a message; the next step then sees the changed fingerprint.
//
// A fix attempt is recorded by observeAttempts (fix_attempt.go) when it
// arrives and told by attemptUpdate, step 3, on the first tick no
// earlier step ends.
var openSteps = []step{
	(*Manager).evidenceStep,
	(*Manager).revisedStep,
	(*Manager).attemptUpdate,
	(*Manager).ackStep,
	(*Manager).reminderStep,
	(*Manager).materialStep,
}

func (m *Manager) open(p *Incident, now time.Time) (Decision, bool) {
	for _, s := range openSteps {
		if d, done := s(m, p, now); done {
			return d, d.Action != 0
		}
	}
	return Decision{}, false
}

func (m *Manager) evidenceStep(p *Incident, now time.Time) (Decision, bool) {
	if m.awaitingEvidence(p, now) {
		return Decision{}, true
	}
	if len(p.Members) == 0 {
		p.State, p.RecoveringSince = Recovering, m.recoveryStart(p, now)
		return Decision{}, true
	}
	return Decision{}, false
}

func (m *Manager) revisedStep(p *Incident, now time.Time) (Decision, bool) {
	if !p.Pending.RevisedOwed() {
		return Decision{}, false
	}
	if now.Before(p.Pending.RevisedDueAt(m.cfg.ReviseSettle)) {
		// Let the new cause settle, so one update carries it
		// with whatever joins it in the next seconds.
		return Decision{}, true
	}
	p.Pending.ClearRevised()
	if causeWithdrawn(p) || hedgesTold(p) {
		// The thread keeps its words; the fingerprint adopts the new
		// root so the move is not re-sent as a material change.
		p.Digest = fingerprint(p)
		return Decision{}, false
	}
	return m.decide(p, Update, ReasonCauseRevised), true
}

// causeWithdrawn reports a revision that took a known cause away and
// found none: the evidence aged out while the failure went on, so the
// incident now stands on its own workload with nothing to add. Saying
// "cause changed: no ready replicas" would retract the cause the thread
// still states, and nothing observed contradicts it. A page always
// speaks.
func causeWithdrawn(p *Incident) bool {
	return p.Pending.CauseReplaced() && p.Cause == nil && p.Tier < Page
}

// hedgesTold reports a revision of an incident that only a listing (a
// roll-up, the startup summary) announced, whose new cause only ties
// with the cause the listing stated: the told root is its rival, so the
// update would say "two possible causes: X or what you were told".
// Nothing observed contradicts the listing, and the update would be the
// incident's first message of its own, a new conversation (and later a
// resolve of its own) for a re-rank. An incident with a thread of its
// own still hears it there. A page always speaks.
func hedgesTold(p *Incident) bool {
	return p.Pending.CauseReplaced() && p.Tier < Page && tiedWithTold(p)
}

// tiedWithTold reports a listed-only incident whose cause ties with the
// root the listing stated. The tie is part of the cause's identity
// (causeIdentity): a skipped hedge adopts it, so the tie ending (the new
// cause alone) is a change, and the thread then learns the new cause.
func tiedWithTold(p *Incident) bool {
	return onlyListed(p) && p.Cause != nil && p.Cause.Rival != nil &&
		p.Pending.WasTold(p.Cause.Rival.Root)
}

// onlyListed reports an incident whose only message so far is the
// listing that carried its announcement.
func onlyListed(p *Incident) bool {
	return p.Delivery.RolledUp() && p.Revision <= 1
}

func (m *Manager) reminderStep(p *Incident, now time.Time) (Decision, bool) {
	if fingerprint(p) != p.Digest {
		return Decision{}, false
	}
	if m.reminderDue(p, now) {
		p.Reminded = now
		return m.decide(p, Update, ReasonReminder), true
	}
	return Decision{}, true
}

func (m *Manager) materialStep(p *Incident, now time.Time) (Decision, bool) {
	if materialHeld(p, now) || m.provisional(p, now) {
		// A provisional rise waits for the saved reasons or the end
		// of the restore grace (restore_reasons.go).
		return Decision{}, true
	}
	p.Delivery.MarkMaterial(now)
	return m.decide(p, Update, ReasonMaterialChange), true
}
