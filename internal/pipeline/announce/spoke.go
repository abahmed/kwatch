package announce

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// maxSpoke bounds the memory of incidents that spoke on their own after
// a listing named them. Past it, after the closed ones are dropped, a
// new one is not remembered: it costs at most one more introduction.
const maxSpoke = 4096

// NoteSpoke records that the message msg for d reached chat as the own
// message of an incident a listing named (a roll-up, the startup or the
// restored summary): it has a thread of its own now. The record is part
// of the startup marker, so a later start does not introduce it again
// (restored_intro.go); the listings' own Followed lists are gone after a
// restart. It keeps the told fingerprint, and drops incidents that are
// over.
func (c *Collector) NoteSpoke(d incident.Decision, msg notification.Message) {
	p := d.Incident
	if d.Action == incident.Resolve || msg.Carrier != "" ||
		msg.PagingOnly || (!p.Delivery.RolledUp() && c.ListedBy(p.ID) == "") {
		return
	}
	s := &c.Startup.Summary
	if told, ok := s.Spoke[p.ID]; ok && told == p.Digest {
		return
	}
	c.pruneSpoke()
	if _, ok := s.Spoke[p.ID]; !ok && len(s.Spoke) >= maxSpoke {
		return
	}
	if s.Spoke == nil {
		s.Spoke = map[string]string{}
	}
	s.Spoke[p.ID] = p.Digest
	c.saveSummary()
}

// pruneSpoke forgets the incidents that are over: resolved, or gone
// with no successor. It reports whether it forgot any.
func (c *Collector) pruneSpoke() bool {
	s := &c.Startup.Summary
	pruned := false
	for id := range s.Spoke {
		if c.env.Incidents.Closed([]string{id}) {
			delete(s.Spoke, id)
			pruned = true
		}
	}
	return pruned
}
