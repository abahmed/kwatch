// Package inventory is kwatch's in-memory model of the cluster: entities,
// the relations between them, their attributes and recent changes.
// In: Observations from sources such as inventory/kube. Out: read-only
// lookups (Reader) used by detection and rootcause. It knows nothing
// about Kubernetes itself.
//
// Identity. An EntityID is group, kind, namespace and name. Kind is the
// lower-case kind rules match on ("pod", "gateway"). Group is empty for
// the built-in group: every Kubernetes built-in API group (core, apps,
// batch, networking.k8s.io and the rest; see kube.GroupFor) and kwatch's
// virtual kinds (registry, zone, cluster-dns, ...). Any other group is
// the object's API group, so kinds that share a name in different groups
// (Istio and Gateway API Gateways, a Knative Service and a core Service)
// are separate entities. Keys render as "kind/namespace/name" in the
// built-in group and "kind.group/namespace/name" otherwise, so built-in
// keys in persisted state, logs and messages never changed.
package inventory
