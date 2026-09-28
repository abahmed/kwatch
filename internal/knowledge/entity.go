package knowledge

import (
	"strings"
	"time"
)

// Kind names an entity type, for example "pod", "node" or "cluster-dns".
// Kinds are lower-case and stable; they appear in persisted state.
type Kind string

// EntityID identifies an entity. Cluster-scoped entities have an empty
// Namespace.
type EntityID struct {
	Kind      Kind
	Namespace string
	Name      string
}

// NewEntityID builds an identifier.
func NewEntityID(kind Kind, namespace, name string) EntityID {
	return EntityID{Kind: kind, Namespace: namespace, Name: name}
}

// String renders the canonical "kind/namespace/name" key.
func (id EntityID) String() string {
	return string(id.Kind) + "/" + id.Namespace + "/" + id.Name
}

// ParseEntityID reverses String.
func ParseEntityID(key string) (EntityID, bool) {
	parts := strings.SplitN(key, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return EntityID{}, false
	}
	return EntityID{
		Kind: Kind(parts[0]), Namespace: parts[1], Name: parts[2],
	}, true
}

// IsZero reports whether the identifier is unset.
func (id EntityID) IsZero() bool {
	return id.Kind == "" && id.Name == ""
}

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
