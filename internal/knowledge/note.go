package knowledge

import "time"

// DefaultMaxNotesPerEntity bounds the notes kept for one entity.
const DefaultMaxNotesPerEntity = 16

// Note is an occurrence reported about an entity, such as a Kubernetes
// Warning event. Notes are evidence: they explain what an entity went
// through even after the event object itself expired.
type Note struct {
	At      time.Time
	Source  string
	Reason  string
	Message string
	Count   int
	Warning bool
}
