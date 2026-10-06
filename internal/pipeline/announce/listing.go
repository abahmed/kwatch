package announce

import (
	"slices"

	"github.com/abahmed/kwatch/internal/incident"
)

// Listing is one message that named several incidents instead of one
// message each: the startup summary, or a roll-up of problems found at
// the same time. It remembers what the message listed, so the first own
// message of a listed incident introduces it and the listing is closed
// once everything it named has resolved.
type Listing struct {
	// Key is the conversation key of the message sent.
	Key string `json:",omitempty"`
	// Incidents lists the incidents the message still waits on.
	Incidents []string `json:",omitempty"`
	// Followed lists the listed incidents that have since sent a
	// message of their own, and Resolved those whose own resolve was
	// sent. A listing without them reads as "nothing sent yet", which
	// costs at most one extra message.
	Followed []string `json:",omitempty"`
	Resolved []string `json:",omitempty"`
}

// StartupState is the persisted startup summary marker, with the
// roll-ups still open. It makes a cold start explicit instead of
// inferring it from an empty incident store. The embedded Listing is the
// startup summary itself; its fields are stored flat, as earlier
// versions wrote them.
type StartupState struct {
	// Complete is true once a startup summary was handed to delivery, or
	// the startup window closed with nothing to report.
	Complete bool
	Listing
	// Rollups are the roll-ups whose incidents have not all resolved.
	Rollups []Listing `json:",omitempty"`
	// Digest is the part of the pending digest no incident record keeps.
	Digest DigestState `json:",omitempty"`
}

// listings returns the startup summary and every open roll-up.
func (s *StartupState) listings() []*Listing {
	out := []*Listing{&s.Listing}
	for i := range s.Rollups {
		out = append(out, &s.Rollups[i])
	}
	return out
}

// Clone copies the state deeply, for a writer on another goroutine.
func (s StartupState) Clone() StartupState {
	s.Listing = s.Listing.clone()
	s.Digest = s.Digest.clone()
	s.Rollups = slices.Clone(s.Rollups)
	for i := range s.Rollups {
		s.Rollups[i] = s.Rollups[i].clone()
	}
	return s
}

func (l Listing) clone() Listing {
	l.Incidents = slices.Clone(l.Incidents)
	l.Followed = slices.Clone(l.Followed)
	l.Resolved = slices.Clone(l.Resolved)
	return l
}

// follow records an own message of an incident the listing named. It
// reports whether d is about such an incident; the decision it returns
// is the one to write the message for: the listing only named the
// incident, so its first own message is a full announcement, not a bare
// update. Its resolve is recorded apart (noteResolved), once it is known
// to have reached chat.
func (l *Listing) follow(d incident.Decision) (incident.Decision, bool) {
	id := d.Incident.ID
	if !slices.Contains(l.Incidents, id) {
		return d, false
	}
	first := !slices.Contains(l.Followed, id)
	if first {
		l.Followed = append(l.Followed, id)
	}
	if first && d.Action == incident.Update {
		d.Action = incident.Announce
	}
	return d, true
}

// noteResolved records that the resolve of listed incident id reached
// chat. It reports whether the listing names the incident.
func (l *Listing) noteResolved(id string) bool {
	if !slices.Contains(l.Incidents, id) {
		return false
	}
	if !slices.Contains(l.Resolved, id) {
		l.Resolved = append(l.Resolved, id)
	}
	return true
}

// closes reports whether the listing is over: every incident it named
// has resolved, and whether people must be told. When every listed
// incident already said it resolved, one more "all resolved" would be a
// repeated recovery.
func (l *Listing) closes(incidents *incident.Manager) (over, tell bool) {
	if len(l.Incidents) == 0 || !incidents.Closed(l.Incidents) {
		return false, false
	}
	return true, !containsAll(l.Resolved, l.Incidents)
}
