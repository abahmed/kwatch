package knowledge

import "time"

// Change is one meaningful modification of an entity: a spec edit, an
// image or data change, a scale, a taint. Status updates and resyncs are
// never changes; sources decide what is meaningful.
type Change struct {
	Entity   EntityID
	At       time.Time
	Actor    string
	Revision string
	Created  bool
	Deleted  bool
	Fields   []FieldChange
}

// FieldChange is one changed path. Before and After are already redacted
// by the source; secret values are represented by hashes.
type FieldChange struct {
	Path   string
	Before string
	After  string
}
