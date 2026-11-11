package incident

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A stale acknowledgement is a kwatch.dev/ack annotation that was on the
// object before the incident opened: someone acknowledged an earlier
// incident and never removed it. It says nothing about this incident, so
// it is ignored: the incident reminds, releases a held page and pages a
// flapper as if the annotation were not there. To acknowledge the new
// incident, change the note or remove the annotation and set it again.
//
// "Before" is read from when kwatch first saw the annotation with its
// present note (the attribute's Since). A restart loses that: the model
// is rebuilt, and every annotation looks new. So the incident remembers
// the objects whose annotation it ignored, and after a restart their
// annotation stays ignored until it changes once the model is back.

// maxAckIgnored bounds the objects remembered per incident. Past it
// the incident remembers only that there were more (ackAll): after a
// restart every annotation seen while the model is rebuilt stays
// ignored, as a listed one would.
const maxAckIgnored = 16

// staleAck reports that the annotation on id, seen since since, is older
// than p and does not acknowledge it. An incident that is acknowledged
// keeps its acknowledgement: this only decides what may start one.
func (m *Manager) staleAck(
	p *Incident, id inventory.EntityID, since time.Time,
) bool {
	if p.Ack != nil {
		return false
	}
	listed := slices.Contains(p.ackIgnored, id)
	known := listed || p.ackAll
	floor := p.Opened
	if known && p.restored && m.restoreGrace.After(floor) {
		// Seen again while the model was rebuilt: still the old one.
		floor = m.restoreGrace
	}
	if since.Before(floor) {
		if !listed {
			p.rememberStaleAck(id)
		}
		return true
	}
	if listed {
		p.ackIgnored = slices.DeleteFunc(slices.Clone(p.ackIgnored),
			func(other inventory.EntityID) bool { return other == id })
	}
	return false
}

// rememberStaleAck notes that the annotation on id was ignored, in the
// list while it has room and as ackAll once it is full.
func (p *Incident) rememberStaleAck(id inventory.EntityID) {
	if len(p.ackIgnored) >= maxAckIgnored {
		p.ackAll = true
		return
	}
	p.ackIgnored = append(slices.Clone(p.ackIgnored), id)
	logLifecycle(p, "ack", "ack annotation ignored: "+
		"older than the incident", "on", describe(id))
}
