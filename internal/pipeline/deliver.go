package pipeline

import (
	"context"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

// heldAnnouncement is an announcement waiting for its investigation.
type heldAnnouncement struct {
	decision incident.Decision
	// decided is when the decision was made; the message is written for
	// that moment, whenever it is sent.
	decided time.Time
	// until is when it is sent without output.
	until time.Time
}

// deliver hands decisions to the sink in order, with the evidence
// investigation found. An announcement, or an update for a material
// change, whose investigation still runs is held for it; every other
// decision goes at once. A decision about a held incident first releases
// what is held, so people never hear an update before the news.
func (a *announcer) deliver(
	ctx context.Context, now time.Time, decisions []incident.Decision,
) {
	for _, d := range decisions {
		a.releaseHeld(ctx, d.Incident.ID)
		if investigable(d) && a.hold(d, now) {
			continue
		}
		a.send(ctx, a.withEvidence(d), now)
		a.forgetEvidence(d)
	}
}

// investigable reports a decision worth fresh evidence: the first
// message, or an update because the incident changed materially. A
// cause revision carries the new cause already, and a resolve needs no
// logs.
func investigable(d incident.Decision) bool {
	return d.Action == incident.Announce ||
		(d.Action == incident.Update &&
			d.Reason == incident.ReasonMaterialChange)
}

// hold holds announcement d until its investigation's result arrives or
// outputWait passes. It reports false when there is nothing to wait for.
func (a *announcer) hold(d incident.Decision, now time.Time) bool {
	if !a.awaitsEvidence(d, now) {
		return false
	}
	if d.Action == incident.Announce {
		// Persist it as not announced until delivery has it, so a
		// restart in the meantime announces it again instead of
		// losing it. A held update changes nothing about that.
		a.incidents.HoldAnnouncement(d.Incident.ID)
	}
	a.held = append(a.held, heldAnnouncement{
		decision: d, decided: now, until: now.Add(outputWait),
	})
	return true
}

// attachOutput keeps a finished investigation's result and sends the
// held announcement waiting for it. A result that comes after its
// announcement went is kept: the next update quotes it if it adds a new
// fact, but it never causes an update on its own.
func (a *announcer) attachOutput(ctx context.Context, r investigationResult) {
	a.pool.received()
	if !a.storeResult(r) {
		return
	}
	if i := a.heldIndexOf(r.id); i >= 0 {
		a.sendHeld(ctx, a.takeHeld(i))
	}
}

// expireHeld sends, without output, every held announcement whose wait
// is over.
func (a *announcer) expireHeld(ctx context.Context, now time.Time) {
	for len(a.held) > 0 && !a.held[0].until.After(now) {
		a.noteLate()
		a.sendHeld(ctx, a.takeHeld(0))
	}
}

// releaseHeld sends the held announcement of incident id, if any, without
// waiting for its output.
func (a *announcer) releaseHeld(ctx context.Context, id string) {
	if i := a.heldIndexOf(id); i >= 0 {
		a.noteLate()
		a.sendHeld(ctx, a.takeHeld(i))
	}
}

// nextHeld is when the oldest held announcement expires, or zero.
func (a *announcer) nextHeld() time.Time {
	if len(a.held) == 0 {
		return time.Time{}
	}
	return a.held[0].until
}

func (a *announcer) heldIndexOf(id string) int {
	for i, h := range a.held {
		if h.decision.Incident.ID == id {
			return i
		}
	}
	return -1
}

// takeHeld removes and returns held announcement i. Announcements are
// held in decision order, so held[0] always expires first.
func (a *announcer) takeHeld(i int) heldAnnouncement {
	h := a.held[i]
	a.held = append(a.held[:i], a.held[i+1:]...)
	return h
}

func (a *announcer) sendHeld(ctx context.Context, h heldAnnouncement) {
	a.incidents.ReleaseAnnouncement(h.decision.Incident.ID)
	a.send(ctx, a.withEvidence(h.decision), h.decided)
}

// send writes the message for d as of at and hands both to the sink.
// A resolve whose fixing change is known says who fixed it and how. The
// first own message of an incident the startup summary listed is
// written as an announcement; the sink still gets the real decision.
func (a *announcer) send(
	ctx context.Context, d incident.Decision, at time.Time,
) {
	a.sink(ctx, d, a.write(a.followSummary(d), at))
}

func (a *announcer) write(
	d incident.Decision, at time.Time,
) notification.Message {
	if fix := d.Incident.FixedBy; d.Action == incident.Resolve && fix != nil {
		return a.messages.WriteResolvedBy(d, at, *fix)
	}
	msg := a.messages.Write(d, at)
	// An incident whose failures another incident took over closes its
	// own alert, but the chat channel already reads about those failures
	// in the other incident's update; a "cause revised" per absorbed
	// incident would be one message per workload in a storm.
	if d.Action == incident.Resolve && d.Incident.SupersededBy != "" {
		msg.PagingOnly = true
	}
	return msg
}

func (a *announcer) noteLate() {
	a.stats.late.Add(1)
	metrics.DefaultRegistry().IncInvestigation("late")
}
