package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Event is one line of the incident timeline.
type Event struct {
	At   time.Time
	Text string
	// Entity is the member the event is about: the one that joined or
	// recovered. Nil for events about the incident itself.
	Entity *inventory.EntityID `json:",omitempty"`
	// Repeats counts earlier entries that said the same thing and were
	// folded into this one to keep the timeline within its bound.
	Repeats int `json:",omitempty"`
	// Gap counts the entries dropped right after this one to keep the
	// timeline within its bound (see trimTimeline). Entries before and
	// after the gap are kept, so the gap marks where history is missing.
	Gap int `json:",omitempty"`
}
