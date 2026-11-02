package incident

import (
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// An acknowledgement is a person saying "I am looking at this" with the
// kwatch.dev/ack annotation on the incident's root or on one of its
// members. While the annotation is there the incident stops reminding:
// the people it would remind are already on it. Everything else carries
// on, because it is news, not a repeat: a worse state, a resolve.
//
// kwatch tells the thread once when the annotation appears and once when
// it goes. It does not name who set it: a Kubernetes object records the
// tool that wrote it last (a field manager), not the person. The
// dashboard shows that tool (Ack.By) when the managed fields name it.

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
	// Since is when kwatch first saw the annotation, or its note change;
	// By is the field manager that wrote it ("kubectl-annotate") when
	// the change history recorded the edit, else empty. Older records
	// have neither.
	Since time.Time `json:",omitzero"`
	By    string    `json:",omitempty"`
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
		if attr, ok := m.model.Attribute(id, kube.AttrAck); ok {
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
func (m *Manager) adoptAck(p *Incident, now time.Time) {
	if found, ok := m.ackOn(p); ok {
		found.AtAnnounce = true
		m.stampAck(&found, now)
		p.Ack = &found
	}
}

// stampAck records when an acknowledgement was seen and who wrote it:
// the field manager the object's managed fields name for the annotation
// (kube.AttrAckBy), else the edit of it in the change history.
func (m *Manager) stampAck(a *Ack, now time.Time) {
	a.Since, a.By = now, ""
	if m.model == nil {
		return
	}
	if by, ok := m.model.Attribute(a.On, kube.AttrAckBy); ok {
		a.By = by.Value.AsText()
		return
	}
	for _, set := range m.model.ChangeSets(a.On, now.Add(-RecentWindow)) {
		for _, change := range set.Changes {
			if change.Entity == a.On && editsAck(change) {
				a.By = change.Actor
			}
		}
	}
}

// editsAck reports a change that wrote the ack annotation.
func editsAck(change inventory.Change) bool {
	for _, f := range change.Fields {
		if strings.Contains(f.Path, kube.AckAnnotation) {
			return true
		}
	}
	return false
}

// ackCarrierGone reports an ack whose entity no longer exists in the
// model, such as a pod replaced by a rollout. Only an entity that still
// exists without the annotation counts as having removed it.
func (m *Manager) ackCarrierGone(p *Incident) bool {
	if m.model == nil || p.Ack == nil {
		return false
	}
	_, ok := m.model.Entity(p.Ack.On)
	return !ok
}

// acked reports an incident whose reminders are held back.
func acked(p *Incident) bool { return p.Ack != nil }

// keepAck takes the same acknowledgement, maybe edited: it is kept and
// not told again.
func (m *Manager) keepAck(ack *Ack, found Ack, now time.Time) {
	if ack.On != found.On || ack.Note != found.Note {
		m.stampAck(ack, now)
	}
	ack.On, ack.Note = found.On, found.Note
}

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
		m.keepAck(p.Ack, found, now)
		return Decision{}, false
	}
	if !has && m.ackCarrierGone(p) {
		// The pod that carried the annotation was replaced, not edited:
		// the acknowledgement stays, and nobody is told it went.
		return Decision{}, false
	}
	audible := p.Tier >= Notify || isPage(p)
	if audible && fingerprint(p) != p.Digest {
		return Decision{}, false
	}
	if has {
		m.stampAck(&found, now)
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
