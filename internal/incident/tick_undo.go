package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// tickSnapshot is what one tick of an incident may change about what
// people were told: its state, the announce and resolve marks, and the
// delivery bookkeeping of the decision it makes.
type tickSnapshot struct {
	state                      State
	resolved, announced        time.Time
	reminded                   time.Time
	reminders                  int
	revision                   int
	digest, headline, alertKey string
	reported                   int
	reportedKnown, held        bool
	sent                       sentMark
	decided                    map[detection.Key]struct{}
	delivery                   Delivery
	pending                    Pending
	route                      *AnnouncedRoute
	// What the steps before the decision change (ackStep, noteAttempt,
	// holdRepeatedPage, the flap count, the timeline notes): left as
	// they were, the next tick would find the news already taken and
	// never tell it.
	tier                 Tier
	ack                  *Ack
	attempt, attemptSeen *inventory.Change
	attemptLate          bool
	timeline             []Event
	cycles               []time.Time
	heldSince            time.Time
}

// snapshotTick records p before its tick. decided is replaced, never
// changed, so keeping the map itself is enough. The same goes for the
// timeline and the cycles: a tick appends to them or replaces them, so
// the slices as they were still hold what they held. The
// acknowledgement is edited in place (keepAck) and is copied.
func snapshotTick(p *Incident) tickSnapshot {
	return tickSnapshot{
		state: p.State, resolved: p.Resolved, announced: p.Announced,
		reminded: p.Reminded, reminders: p.Reminders,
		revision: p.Revision, digest: p.Digest,
		headline: p.Headline, alertKey: p.AlertKey,
		reported: p.Reported, reportedKnown: p.ReportedKnown,
		held: p.Held, sent: p.sent, decided: p.decided,
		delivery: p.Delivery, pending: p.Pending, route: p.AnnouncedRoute,
		tier: p.Tier, ack: p.Ack.clone(), attempt: p.Attempt,
		attemptSeen: p.attemptSeen, attemptLate: p.attemptLate,
		timeline: p.Timeline, cycles: p.Cycles, heldSince: p.heldSince,
	}
}

// undoTick takes back a tick that panicked before its decision reached
// the caller: the incident goes back to where it was, so the next tick
// decides it again and the message is not lost, and nothing believes it
// was sent. A quiet resolve has no message to lose and stays.
func (m *Manager) undoTick(p *Incident, s tickSnapshot) {
	if m.incidents[p.ID] != p || (p.State == Resolved && p.quiet) {
		return
	}
	p.State, p.Resolved, p.Announced = s.state, s.resolved, s.announced
	p.Reminded, p.Reminders = s.reminded, s.reminders
	p.Revision, p.Digest = s.revision, s.digest
	p.Headline, p.AlertKey, p.Held = s.headline, s.alertKey, s.held
	p.Reported, p.ReportedKnown = s.reported, s.reportedKnown
	p.sent, p.decided, p.Delivery = s.sent, s.decided, s.delivery
	p.Pending, p.AnnouncedRoute = s.pending, s.route
	p.Tier, p.Ack, p.heldSince = s.tier, s.ack, s.heldSince
	p.Attempt, p.attemptSeen = s.attempt, s.attemptSeen
	p.attemptLate = s.attemptLate
	p.Timeline, p.Cycles = s.timeline, s.cycles
}
