package pipeline

import (
	"context"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// jointRecovery is the carrier of a resolve that another resolve of the
// same tick names.
const jointRecovery = "joint recovery"

// jointMin is how many such resolves of one tick are sent as one message.
const jointMin = 2

// jointRecoverable reports a resolve of an incident that only a roll-up
// named: it never spoke on its own, so no thread of its own is left to
// close, and the chat gets it as a reply under the roll-up. A page, a
// resolve for the pagers, a handover and the resolve of a superseded
// incident close something and stay on their own.
//
// A member whose page was held beside the one page that stood for its
// listing (PageHeld) never opened an alert, so its resolve joins too,
// unless it has spoken in a thread of its own since the listing named it.
func (a *announcer) jointRecoverable(d incident.Decision) bool {
	p := d.Incident
	return d.Action == incident.Resolve && p.Delivery.RolledUp() &&
		p.Tier < incident.Page && !p.Delivery.OpenAtPagers() &&
		p.SupersededBy == "" && !d.Unannounced && !d.Handover &&
		!d.Thread && !d.PagedAlready && !a.collect.SpokeOnItsOwn(p.ID)
}

// splitRecoveries takes the follower resolves out of one tick's decisions.
// The first jointRecoverable resolve leads and the others, in order, are
// kept in a.joint under its incident ID; the rest of decisions pass on.
func (a *announcer) splitRecoveries(
	decisions []incident.Decision,
) []incident.Decision {
	var group []incident.Decision
	for _, d := range decisions {
		if a.jointRecoverable(d) {
			group = append(group, d)
		}
	}
	if len(group) < jointMin {
		return decisions
	}
	lead := group[0].Incident.ID
	a.joint = map[string][]incident.Decision{lead: group[1:]}
	rest := make([]incident.Decision, 0, len(decisions)-len(group)+1)
	for _, d := range decisions {
		if !a.jointRecoverable(d) || d.Incident.ID == lead {
			rest = append(rest, d)
		}
	}
	return rest
}

// withRecovered adds the recoveries a.joint holds for lead to its message
// and records them as carried by it: the audit log keeps each decision,
// delivery sends the one message. The lead's own resolve is delivered as
// usual by send.
//
// Nothing is recorded until the joint message is composed. If composing
// panics, the lead goes alone, the followers stay in a.joint and flushJoint
// sends each of them on its own, so no resolve is lost.
func (a *announcer) withRecovered(
	ctx context.Context, lead incident.Decision, msg notification.Message,
	at time.Time,
) notification.Message {
	followers := a.joint[lead.Incident.ID]
	written := make([]incident.Decision, 0, len(followers))
	others := make([]notification.Message, 0, len(followers))
	var joint notification.Message
	ok := contain("compose", "joint", func() {
		for _, f := range followers {
			f = a.withKindNames(a.withChanges(f, at))
			written = append(written, f)
			others = append(others, a.write(f, at))
		}
		joint = compose.Recovered(msg, others)
	})
	if !ok {
		// The lead goes alone; flushJoint sends the followers.
		return msg
	}
	delete(a.joint, lead.Incident.ID)
	for i, f := range written {
		m := others[i]
		m.Carrier = jointRecovery
		m.SkipPaging = true
		a.collect.NoteResolveDelivered(f.Incident.ID)
		a.forgetEvidence(f)
		a.sink(ctx, f, m)
	}
	return joint
}

// flushJoint sends, as resolves of their own, the followers that no lead
// carried: the lead's message failed to compose, or the lead never reached
// send. It ends the tick's joint recovery, so a.joint is empty afterwards.
func (a *announcer) flushJoint(ctx context.Context, at time.Time) {
	left := a.joint
	a.joint = nil
	leads := make([]string, 0, len(left))
	for lead := range left {
		leads = append(leads, lead)
	}
	sort.Strings(leads)
	for _, lead := range leads {
		for _, f := range left[lead] {
			contain("compose", "write", func() {
				a.send(ctx, a.withEvidence(f), at)
			})
			a.forgetEvidence(f)
		}
	}
}
