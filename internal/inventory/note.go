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
}

// earliest returns the earlier non-zero time of a and b.
func earliest(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}
