package inventory

// nodeKind is the kind of a Kubernetes Node, spelled here because the
// model does not import the Kubernetes source. A Node owns only the
// static pods of its kubelet.
const nodeKind Kind = "node"

func isNode(id EntityID) bool {
	return id.Group == "" && id.Kind == nodeKind
}

// Owner is the entity that owns id: the first target of its owned-by
// relation. It reports false for an entity without an owner.
//
// An owner reference names no namespace, so the source files an owner
// of a kind it does not know to be cluster-scoped under the namespace
// of the owned object. When no such entity exists but a cluster-scoped
// one of that group, kind and name does, that one is the owner: a pod
// or DaemonSet owned by a cluster-scoped custom resource.
func Owner(model Reader, id EntityID) (EntityID, bool) {
	owners := model.Related(id, OwnedBy, Outgoing)
	if len(owners) == 0 {
		return EntityID{}, false
	}
	return ResolveOwner(model, owners[0]), true
}

// ResolveOwner is the entity an owned-by target really names: the
// cluster-scoped entity of that group, kind and name when the target
// as filed does not exist but that one does, else the target itself.
func ResolveOwner(model Reader, owner EntityID) EntityID {
	if owner.Namespace == "" || model.Exists(owner) {
		return owner
	}
	cluster := owner
	cluster.Namespace = ""
	if model.Exists(cluster) {
		return cluster
	}
	return owner
}
