package inventory

import "time"

// Attribute is one observed property of an entity. Since is when the
// current value was first observed; Updated is the latest observation.
type Attribute struct {
	Value   Value
	Since   time.Time
	Updated time.Time
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

func (e Entity) clone() Entity {
	out := e
	out.Attributes = make(map[string]Attribute, len(e.Attributes))
	for name, attribute := range e.Attributes {
		out.Attributes[name] = attribute
	}
	return out
}
