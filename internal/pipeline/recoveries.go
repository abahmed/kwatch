package pipeline

import (
	"context"
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
func jointRecoverable(d incident.Decision) bool {
	p := d.Incident
	return d.Action == incident.Resolve && p.Delivery.RolledUp() &&
		p.Tier < incident.Page && !p.Delivery.OpenAtPagers() &&
		!p.Delivery.PageHeld() && p.SupersededBy == "" &&
		!d.Unannounced && !d.Handover && !d.Thread && !d.PagedAlready
}

// splitRecoveries takes the follower resolves out of one tick's decisions.
// The first jointRecoverable resolve leads and the others, in order, are
// kept in a.joint under its incident ID; the rest of decisions pass on.
func (a *announcer) splitRecoveries(
	decisions []incident.Decision,
) []incident.Decision {
	var group []incident.Decision
	for _, d := range decisions {
		if jointRecoverable(d) {
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
		if !jointRecoverable(d) || d.Incident.ID == lead {
			rest = append(rest, d)
		}
	}
	return rest
}

// withRecovered adds the recoveries a.joint holds for lead to its message
// and records them as carried by it: the audit log keeps each decision,
// delivery sends the one message. The lead's own resolve is delivered as
// usual by send.
func (a *announcer) withRecovered(
	ctx context.Context, lead incident.Decision, msg notification.Message,
	at time.Time,
) notification.Message {
	followers := a.joint[lead.Incident.ID]
	delete(a.joint, lead.Incident.ID)
	others := make([]notification.Message, 0, len(followers))
	for _, f := range followers {
		f = a.withKindNames(a.withChanges(f, at))
		m := a.write(f, at)
		others = append(others, m)
		m.Carrier = jointRecovery
		m.SkipPaging = true
		a.collect.NoteResolveDelivered(f.Incident.ID)
		a.forgetEvidence(f)
		a.sink(ctx, f, m)
	}
	return compose.Recovered(msg, others)
}
