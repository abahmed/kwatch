package inventory

import "strings"

// Kind names an entity type, for example "pod", "node" or "cluster-dns".
// Kinds are lower-case and stable; they appear in persisted state.
type Kind string

// EntityID identifies an entity by group, kind, namespace and name.
// Cluster-scoped entities have an empty Namespace. Group is empty for
// the built-in group, which holds every Kubernetes built-in kind and
// kwatch's virtual kinds; any other group is an API group such as
// "gateway.networking.k8s.io". The empty group is omitted from JSON so
// records written before groups existed decode unchanged.
type EntityID struct {
	Group     string `json:",omitempty"`
	Kind      Kind
	Namespace string
	Name      string
}

// NewEntityID builds an identifier in an API group.
func NewEntityID(group string, kind Kind, namespace, name string) EntityID {
	return EntityID{
		Group: group, Kind: kind, Namespace: namespace, Name: name,
	}
}

// CoreID builds an identifier in the built-in group.
func CoreID(kind Kind, namespace, name string) EntityID {
	return EntityID{Kind: kind, Namespace: namespace, Name: name}
}

// String renders the canonical key: "kind/namespace/name" in the
// built-in group and "kind.group/namespace/name" in any other group.
// Built-in keys never change, so they are safe in persisted state.
func (id EntityID) String() string {
	return id.QualifiedKind() + "/" + id.Namespace + "/" + id.Name
}

// QualifiedKind is the kind, suffixed with ".group" outside the built-in
// group, as in "gateway.gateway.networking.k8s.io".
func (id EntityID) QualifiedKind() string {
	if id.Group == "" {
		return string(id.Kind)
	}
	return string(id.Kind) + "." + id.Group
}

// ParseEntityID reverses String. Kinds never contain a dot, so the first
// dot of the first segment separates the kind from its group.
func ParseEntityID(key string) (EntityID, bool) {
	parts := strings.SplitN(key, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return EntityID{}, false
	}
	kind, group, qualified := strings.Cut(parts[0], ".")
	if kind == "" || (qualified && group == "") {
		return EntityID{}, false
	}
	return NewEntityID(group, Kind(kind), parts[1], parts[2]), true
}

// IsZero reports whether the identifier is unset.
func (id EntityID) IsZero() bool {
	return id.Kind == "" && id.Name == ""
}
