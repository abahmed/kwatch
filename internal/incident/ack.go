package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An acknowledgement is a person saying "I am looking at this" with the
// kwatch.io/ack annotation on the incident's root or on one of its
// members. While the annotation is there the incident stops reminding:
// the people it would remind are already on it. Everything else carries
// on, because it is news, not a repeat: a worse state, a resolve.
//
// kwatch tells the thread once when the annotation appears and once when
// it goes. It does not name who set it: a Kubernetes object records the
// tool that wrote it last (a field manager), not the person.

// Reasons of the two updates an acknowledgement makes.
const (
	// ReasonAcknowledged is the update that says someone is on it.
	ReasonAcknowledged Reason = "acknowledged"
	// ReasonAckRemoved is the update that says nobody is any more.
	ReasonAckRemoved Reason = "acknowledgement removed"
)

// Ack is the acknowledgement of an incident.
type Ack struct {
	// On is the object that carries the annotation.
	On inventory.EntityID
	// Note is the annotation value: a person's text, already bounded
	// and stripped of credentials.
	Note string
	// AtAnnounce is set when the annotation was already there when the
	// incident was announced. The announcement says so, and no separate
	// update is sent.
	AtAnnounce bool `json:",omitempty"`
}

func (a *Ack) clone() *Ack {
	if a == nil {
		return nil
	}
	out := *a
	return &out
}

// ackOn finds the annotation on the root, then on the members.
func (m *Manager) ackOn(p *Incident) (Ack, bool) {
	if m.model == nil {
		return Ack{}, false
	}
	members := make([]inventory.EntityID, 0, len(p.Members))
	for _, f := range p.Members {
		members = append(members, f.Entity)
	}
	slices.SortFunc(members, compareIDs)
	for _, id := range append([]inventory.EntityID{p.Root}, members...) {
		entity, ok := m.model.Entity(id)
		if !ok {
			continue
		}
		if attr, ok := entity.Attribute(kube.AttrAck); ok {
			return Ack{On: id, Note: attr.Value.AsText()}, true
		}
	}
	return Ack{}, false
}

func compareIDs(a, b inventory.EntityID) int {
	switch {
	case a.String() < b.String():
		return -1
	case a.String() > b.String():
		return 1
	}
	return 0
}

// adoptAck records an annotation that is already there when p is
// announced. The announcement mentions it.
func (m *Manager) adoptAck(p *Incident) {
	if found, ok := m.ackOn(p); ok {
		found.AtAnnounce = true
		p.Ack = &found
	}
}

// acked reports an incident whose reminders are held back.
func acked(p *Incident) bool { return p.Ack != nil }

// ackStep tells the thread that an acknowledgement appeared or went. It
// waits while a material change is not yet told, so no update swallows
// news. An incident nobody was messaged about (a digest line) takes the
// state without a message. A restored incident waits for the model to be
// rebuilt, or its annotation would look removed.
func (m *Manager) ackStep(p *Incident, now time.Time) (Decision, bool) {
	found, has := m.ackOn(p)
	if p.Held || m.inGrace(p, now) || (!has && p.Ack == nil) {
		return Decision{}, false
	}
	if has && p.Ack != nil {
		// Same acknowledgement, maybe edited: kept, not told again.
		p.Ack.On, p.Ack.Note = found.On, found.Note
		return Decision{}, false
	}
	audible := p.Tier >= Notify || isPage(p)
	if audible && fingerprint(p) != p.Digest {
		return Decision{}, false
	}
	if has {
		p.Ack = &found
		if audible {
			p.note(now, "acknowledged on "+describe(found.On))
			return m.decide(p, Update, ReasonAcknowledged), true
		}
		return Decision{}, false
	}
	p.Ack = nil
	// Reminders count from now, not from before the acknowledgement.
	p.Reminded = now
	if audible {
		p.note(now, "acknowledgement removed")
		return m.decide(p, Update, ReasonAckRemoved), true
	}
	return Decision{}, false
}
