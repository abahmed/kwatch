package pipeline

import (
	"context"
	"slices"
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
		// losing it.
		a.incidents.HoldAnnouncement(d.Incident.ID)
	} else {
		// A held update is lost by a restart too; the record keeps
		// the fingerprint from before it so it is decided again.
		a.incidents.HoldUpdate(d.Incident.ID)
	}
	a.held = append(a.held, heldAnnouncement{
		decision: d, decided: now, until: now.Add(outputWait),
	})
	return true
}

// attachOutput keeps a finished investigation's result and sends the
// held announcement waiting for it. A result that comes after its
// announcement went is kept: the next update quotes it if it adds a new
// fact, but it never causes an update on its own. It reports whether it
// released an announcement, which changes the incident's saved state.
func (a *announcer) attachOutput(
	ctx context.Context, r investigationResult,
) bool {
	a.pool.received()
	if !a.storeResult(r) {
		return false
	}
	if i := a.heldIndexOf(r.id); i >= 0 {
		return a.sendHeld(ctx, a.takeHeld(i))
	}
	return false
}

// expireHeld sends, without output, every held announcement whose wait
// is over. It reports whether it sent one, which changes the incident's
// saved state.
func (a *announcer) expireHeld(ctx context.Context, now time.Time) bool {
	sent := false
	for len(a.held) > 0 && !a.held[0].until.After(now) {
		a.noteLate()
		sent = a.sendHeld(ctx, a.takeHeld(0)) || sent
	}
	return sent
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

// dropHeld forgets the held announcements of incidents that resolved
// without a message: they have no story to start.
func (a *announcer) dropHeld(ids []string) {
	a.held = slices.DeleteFunc(a.held, func(h heldAnnouncement) bool {
		return slices.Contains(ids, h.decision.Incident.ID)
	})
}

// sendHeld sends a held decision and reports whether it did: an
// incident that resolved without a message has nothing to say.
func (a *announcer) sendHeld(ctx context.Context, h heldAnnouncement) bool {
	if a.incidents.ResolvedQuietly(h.decision.Incident.ID) {
		return false
	}
	a.incidents.ReleaseAnnouncement(h.decision.Incident.ID)
	a.incidents.ReleaseUpdate(h.decision.Incident.ID)
	a.send(ctx, a.withEvidence(h.decision), h.decided)
	return true
}

// send writes the message for d as of at and hands both to the sink.
// A resolve whose fixing change is known says who fixed it and how. The
// first own message of an incident the startup summary listed is
// written as an announcement; the sink still gets the real decision.
func (a *announcer) send(
	ctx context.Context, d incident.Decision, at time.Time,
) {
	d = a.withWake(a.withKindNames(a.withChanges(d, at)), at)
	msg := a.write(a.collect.Follow(d), at)
	a.scopePaging(d, &msg)
	if d.PagedAlready {
		msg.SkipPaging = true
	}
	if d.Action == incident.Resolve && msg.Carrier == "" && !msg.PagingOnly {
		a.collect.NoteResolveDelivered(d.Incident.ID)
	}
	a.sink(ctx, d, msg)
}

// scopePaging keeps the paging providers' view of an incident whole: any
// message of its own reaches them and is remembered, and a resolve goes to
// them only when an announcement did. A resolve of an incident that only a
// digest, roll-up or summary carried would close an alert nobody opened.
func (a *announcer) scopePaging(
	d incident.Decision, msg *notification.Message,
) {
	id := d.Incident.ID
	if d.Action != incident.Resolve {
		if pageHeldBack(d) {
			// The page of this outage already went out and its resolve
			// closed it. Every later message (failing again, reminders,
			// material changes) is chat news; the pagers hear of it
			// again only when the tier itself rises to Page.
			msg.SkipPaging = true
			return
		}
		a.incidents.RecordPaged(id, true)
		return
	}
	if a.incidents.Paged(id) {
		a.incidents.RecordPaged(id, false)
		return
	}
	msg.SkipPaging = true
	if msg.PagingOnly {
		// Paging was the only audience and it never heard of the
		// incident: nobody receives this, the audit log still records it.
		msg.PagingOnly = false
		msg.Carrier = "unannounced"
	}
}

// pageHeldBack reports a message of an incident whose page was held at
// notify (PageHeld) because the page of the same outage already went out.
// That page stays the one page of the outage: the incident pages again
// only when its tier is Page again.
func pageHeldBack(d incident.Decision) bool {
	return d.Action != incident.Resolve &&
		d.Incident.Delivery.PageHeld() && d.Incident.Tier != incident.Page
}

func (a *announcer) write(
	d incident.Decision, at time.Time,
) notification.Message {
	if fix := d.Incident.FixedBy; d.Action == incident.Resolve && fix != nil {
		return a.messages.WriteResolvedBy(d, at, *fix)
	}
	msg := a.messages.Write(d, at)
	// An incident whose failures another incident took over closes like
	// any other: its thread is edited to say "Moved: ...", and the
	// paging providers close the alert they opened. One a roll-up
	// carried has no thread of its own, and one nobody heard of has
	// none at all: those close for the alert-tracking providers alone.
	if d.Action == incident.Resolve && (d.Unannounced ||
		(d.Incident.SupersededBy != "" && !d.Handover &&
			d.Incident.Delivery.RolledUp())) {
		msg.PagingOnly = true
	}
	return msg
}

func (a *announcer) noteLate() {
	a.stats.late.Add(1)
	metrics.DefaultRegistry().IncInvestigation("late")
}
