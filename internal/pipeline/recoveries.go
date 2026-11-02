package pipeline

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// jointRecovery is the carrier of a resolve that another resolve of the
// same tick names.
const jointRecovery = "joint recovery"

// jointMin is how many such resolves of one tick are sent as one message.
const jointMin = 2

// jointEnd is how a resolve that may share a message ended. Only
// resolves that ended the same way share one.
type jointEnd int

const (
	// alone is a resolve that is sent on its own.
	alone jointEnd = iota
	// jointRecovered is a recovery, sent under "Recovered at the same
	// moment".
	jointRecovered
	// jointDeleted is the end of a deleted object or namespace, sent
	// under "Deleted at the same moment".
	jointDeleted
)

// jointGroup is what the lead resolve of one kind of end carries.
type jointGroup struct {
	end       jointEnd
	followers []incident.Decision
}

// jointKind reports how a resolve of an incident that only a roll-up
// named ended, or alone for any other resolve. Such an incident never
// spoke on its own, so no thread of its own is left to close, and the
// chat gets it as a reply under the roll-up. A page, a resolve for the
// pagers, a handover and the resolve of a superseded incident close
// something and stay on their own. So does a resolve at the still-broken
// cap: it did not recover, and it was not deleted either.
//
// A member whose page was held beside the one page that stood for its
// listing (PageHeld) never opened an alert, so its resolve joins too,
// unless it has spoken in a thread of its own since the listing named it.
func (a *announcer) jointKind(d incident.Decision) jointEnd {
	p := d.Incident
	if d.Action != incident.Resolve || !p.Delivery.RolledUp() ||
		p.Tier >= incident.Page || p.Delivery.OpenAtPagers() ||
		p.SupersededBy != "" || d.Unannounced || d.Handover ||
		d.Thread || d.PagedAlready || a.collect.SpokeOnItsOwn(p.ID) {
		return alone
	}
	return a.endOf(d)
}

// endOf reports whether a resolve ended in a recovery, in the deletion
// of what its message speaks of, or neither. A resolve that a listing
// named speaks of the subject the listing named (Decision.AsAnnounced);
// any other speaks of its root. One where something else was deleted,
// such as a member, is neither: its message may say "healthy again"
// while the end was a deletion, so it is named under no heading.
func (a *announcer) endOf(d incident.Decision) jointEnd {
	p := d.Incident
	switch {
	case strings.HasPrefix(string(d.Reason),
		incident.StoppedTrackingPrefix):
		return alone
	case len(p.Removed) == 0 && !p.NamespaceRemoved:
		return jointRecovered
	case !p.Subject.IsZero() && a.listedOnly(d, false) != "" &&
		deleted(p, p.Subject):
		return jointDeleted
	case deleted(p, p.Root) &&
		(p.Subject.IsZero() || deleted(p, p.Subject)):
		return jointDeleted
	}
	return alone
}

// deleted reports that id ended because it was deleted, alone or with
// its namespace, as the resolve message says "was deleted". A node has
// its own wording ("is gone"), so it never counts.
func deleted(p incident.Incident, id inventory.EntityID) bool {
	if id.Kind == kube.KindNode {
		return false
	}
	return slices.Contains(p.Removed, id) ||
		(p.NamespaceRemoved && id.Namespace == p.Root.Namespace)
}

// splitRecoveries takes the follower resolves out of one tick's decisions.
// Resolves are grouped by how they ended (jointKind); in each group of at
// least jointMin the first leads and the others, in order, are kept in
// a.joint under its incident ID. The rest of decisions pass on.
func (a *announcer) splitRecoveries(
	decisions []incident.Decision,
) []incident.Decision {
	groups := map[jointEnd][]incident.Decision{}
	for _, d := range decisions {
		if end := a.jointKind(d); end != alone {
			groups[end] = append(groups[end], d)
		}
	}
	followers := map[string]bool{}
	for end, group := range groups {
		if len(group) < jointMin {
			continue
		}
		if a.joint == nil {
			a.joint = map[string]jointGroup{}
		}
		a.joint[group[0].Incident.ID] = jointGroup{
			end: end, followers: group[1:]}
		for _, f := range group[1:] {
			followers[f.Incident.ID] = true
		}
	}
	if len(followers) == 0 {
		return decisions
	}
	rest := make([]incident.Decision, 0, len(decisions)-len(followers))
	for _, d := range decisions {
		if !followers[d.Incident.ID] {
			rest = append(rest, d)
		}
	}
	return rest
}

// joinedBy is the composer that lists the followers of a lead whose
// resolve ended as end.
func joinedBy(end jointEnd) func(
	notification.Message, []notification.Message,
) notification.Message {
	if end == jointDeleted {
		return compose.Deleted
	}
	return compose.Recovered
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
	group := a.joint[lead.Incident.ID]
	followers := group.followers
	written := make([]incident.Decision, 0, len(followers))
	others := make([]notification.Message, 0, len(followers))
	var joint notification.Message
	ok := contain("compose", "joint", func() {
		for _, f := range followers {
			f = a.withKindNames(a.withChanges(f, at))
			// A follower never spoke on its own (jointKind), so
			// it speaks of what its listing named, like send's resolves.
			f.AsAnnounced = a.listedOnly(f, false) != ""
			written = append(written, f)
			others = append(others, a.write(f, at))
		}
		joint = joinedBy(group.end)(msg, others)
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
		for _, f := range left[lead].followers {
			contain("compose", "write", func() {
				a.send(ctx, a.withEvidence(f), at)
			})
			a.forgetEvidence(f)
		}
	}
}
