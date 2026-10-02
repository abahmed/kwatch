package inventory

import "time"

// Change is one meaningful modification of an entity: a spec edit, an
// image or data change, a scale, a taint. Status updates and resyncs are
// never changes; sources decide what is meaningful.
type Change struct {
	Entity EntityID
	// At is when the change happened, from the API server when the
	// source knows it (the writer's managed-fields time or the creation
	// time), otherwise when kwatch received it.
	At time.Time
	// Observed is when kwatch received the change. It is zero for
	// changes recorded before it existed; read ReceivedAt instead.
	Observed time.Time `json:",omitempty"`
	// Actor is the field manager of the latest spec write, such as
	// "kubectl-client-side-apply" or "argocd-controller".
	Actor string
	// App names the GitOps application or release that owns the object,
	// such as "argocd/shop" or "helm/api". Empty when none is known.
	App string `json:",omitempty"`
	// Revision is the object's revision after the change: a Deployment
	// revision, a controller revision, a generation or a data hash.
	Revision string
	Created  bool
	Deleted  bool
	Fields   []FieldChange
}

// ReceivedAt is when kwatch received the change, falling back to At for
// changes without a receive time.
func (c Change) ReceivedAt() time.Time {
	if c.Observed.IsZero() {
		return c.At
	}
	return c.Observed
}

// FieldChange is one changed path. Before and After are already redacted
// by the source; secret values are represented by hashes.
type FieldChange struct {
	Path   string
	Before string
	After  string
}
