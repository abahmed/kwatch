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
}

// maxNoteOrigins bounds the origins remembered for one note.
const maxNoteOrigins = 32

// noteKey identifies the notes that merge into one.
type noteKey struct{ source, reason, uid string }

// originCount is the count one origin contributed to a note.
type originCount struct {
	origin string
	count  int
}

// addOrigin records the count of origin among parts, newest last, and
// returns the new parts and their total.
func addOrigin(
	parts []originCount, origin string, count int,
) ([]originCount, int) {
	kept := parts[:0:0]
	for _, part := range parts {
		if part.origin != origin {
			kept = append(kept, part)
		}
	}
	kept = append(kept, originCount{origin, count})
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
