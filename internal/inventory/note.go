package inventory

import "time"

// DefaultMaxNotesPerEntity bounds the notes kept for one entity.
const DefaultMaxNotesPerEntity = 16

// Note is an occurrence reported about an entity, such as a Kubernetes
// Warning event. Notes are evidence: they explain what an entity went
// through even after the event object itself expired.
type Note struct {
	// At is when the occurrence was last seen.
	At time.Time
	// FirstSeen is when it was first seen. A repeated note keeps the
	// first time of the note it replaces.
	FirstSeen time.Time
	Source    string
	Reason    string
	Message   string
	Count     int
	Warning   bool
	// UID is the UID of the object the note is about. A note whose UID
	// differs from its entity's current UID belongs to an older object
	// that had the same name, and is not evidence about this one.
	UID string
	// Origin names the report the note came from, such as the Event
	// object. Notes of one source, reason and UID with different origins
	// add their counts; the same origin again replaces its own count.
	Origin string
	// Refusal is the registry's own words when the note is a failed
	// image pull the registry refused, such as "unauthorized:
	// authentication required". Message is cut to a bounded length and
	// the refusal comes last, so it is kept apart.
	Refusal string
	// Recent lists the reports behind Count with when each was last
	// seen, oldest first and bounded by maxNoteOrigins, so a reader can
	// count what happened inside a window. Treat it as read-only.
	Recent []Occurrence
}

// Occurrence is one report behind a note: one Event object, or one
// sighting of a note without an origin.
type Occurrence struct {
	At    time.Time
	Count int
}

// CountSince adds up the occurrences seen at or after since.
func (n Note) CountSince(since time.Time) int {
	total := 0
	for _, seen := range n.Recent {
		if !seen.At.Before(since) {
			total += max(seen.Count, 1)
		}
	}
	return total
}

// maxNoteOrigins bounds the origins remembered for one note.
const maxNoteOrigins = 32

// noteKey identifies the notes that merge into one.
type noteKey struct{ source, reason, uid string }

// originCount is the count one origin contributed to a note.
type originCount struct {
	origin string
	count  int
	at     time.Time
}

// addOrigin records the count of origin among parts, newest last, and
// returns the new parts and their total.
func addOrigin(
	parts []originCount, origin string, count int, at time.Time,
) ([]originCount, int) {
	kept := parts[:0:0]
	for _, part := range parts {
		if part.origin != origin {
			kept = append(kept, part)
		}
	}
	kept = append(kept, originCount{origin, count, at})
	if over := len(kept) - maxNoteOrigins; over > 0 {
		kept = kept[over:]
	}
	total := 0
	for _, part := range kept {
		total += part.count
	}
	return kept, total
}

// earliest returns the earlier non-zero time of a and b.
func earliest(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

// occurrences lists the parts as a note's Recent.
func occurrences(parts []originCount) []Occurrence {
	out := make([]Occurrence, 0, len(parts))
	for _, part := range parts {
		out = append(out, Occurrence{At: part.at, Count: part.count})
	}
	return out
}

// sighted returns recent with one more sighting, in a new slice: readers
// of the model hold the old one. A higher count than the latest sighting
// is that same Event counting up, so it takes the latest one's place.
func sighted(recent []Occurrence, seen Occurrence) []Occurrence {
	if n := len(recent); n > 0 && seen.Count > recent[n-1].Count {
		recent = recent[:n-1]
	}
	if over := len(recent) + 1 - maxNoteOrigins; over > 0 {
		recent = recent[over:]
	}
	return append(recent[:len(recent):len(recent)], seen)
}
