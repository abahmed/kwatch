package inventory

import "time"

// Attribute is one observed property of an entity. Since is when the
// current value was first observed; Updated is the latest observation.
type Attribute struct {
	Value   Value
	Since   time.Time
	Updated time.Time
	// Flips are the times a boolean value changed, oldest first, at
	// most MaxAttributeFlips of them. The first value seen is not a
	// flip. Other kinds of value keep none.
	Flips []time.Time
}

// MaxAttributeFlips bounds the flips kept for one attribute, so a value
// that changes every second costs a fixed amount of memory.
const MaxAttributeFlips = 16

// withFlip returns the flips after a change at at. It copies, because
// entity snapshots share the earlier slice.
func withFlip(flips []time.Time, at time.Time) []time.Time {
	if len(flips) >= MaxAttributeFlips {
		flips = flips[len(flips)-MaxAttributeFlips+1:]
	}
	out := make([]time.Time, 0, len(flips)+1)
	return append(append(out, flips...), at)
}

// Entity is a snapshot of one entity. Snapshots are detached copies and
// safe to keep after the Model changes.
type Entity struct {
	ID         EntityID
	UID        string
	FirstSeen  time.Time
	Attributes map[string]Attribute
}

// Attribute returns the named attribute.
func (e Entity) Attribute(name string) (Attribute, bool) {
	attribute, ok := e.Attributes[name]
	return attribute, ok
}
