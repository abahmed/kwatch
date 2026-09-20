package model

import (
	"sort"
	"strings"
)

// An ObjectRef points at one Kubernetes object: the thing an incident, a
// change, or a graph node is about.
//
// The triple was previously assembled by hand wherever it was needed --
// "kind/namespace/name" appears in the insight engine, the resource graph, the
// storage graph, the report builder and the pattern matcher -- and the copies
// did not agree. Workload incidents store Name as "namespace/name", so one
// site trimmed that prefix and another did not, producing
// "deployment/ns/ns/name"; keys built by one side then failed to match keys
// built by the other, and the only symptom was a diagnosis that quietly found
// no dependencies. One type with one formatter removes the class of bug.
type ObjectRef struct {
	Kind      string
	Namespace string
	Name      string
}

// ObjectKey is the canonical string form: "kind/namespace/name". Cluster
// scoped objects have an empty namespace, which yields "node//name" -- the
// empty middle segment is deliberate, so the arity of a key never varies.
func ObjectKey(kind, namespace, name string) string {
	return kind + "/" + namespace + "/" + name
}

// Key returns the canonical string form of the reference.
func (r ObjectRef) Key() string {
	return ObjectKey(r.Kind, r.Namespace, r.Name)
}

// NewObjectRef builds a reference, accepting the "namespace/name" spelling
// that workload incidents carry in Name and reducing it to the bare name.
func NewObjectRef(kind, namespace, name string) ObjectRef {
	if namespace != "" {
		name = strings.TrimPrefix(name, namespace+"/")
	}
	return ObjectRef{Kind: kind, Namespace: namespace, Name: name}
}

// ParseObjectKey splits a canonical key back into its parts. ok reports
// whether the key had the full three segments; a caller handed a truncated
// key gets what could be read plus ok=false, so a malformed key is skipped
// rather than rendered as a half-named object.
//
// A name may not contain "/", so the split is unambiguous for well-formed
// keys.
func ParseObjectKey(key string) (ref ObjectRef, ok bool) {
	parts := strings.SplitN(key, "/", 3)
	switch len(parts) {
	case 3:
		return ObjectRef{
			Kind:      parts[0],
			Namespace: parts[1],
			Name:      parts[2],
		}, true
	case 2:
		return ObjectRef{Kind: parts[0], Namespace: parts[1]}, false
	default:
		return ObjectRef{Kind: key}, false
	}
}

// Describe renders the object the way alert text refers to it: "deployment
// ns/name", or "node name" for a cluster-scoped object.
func (r ObjectRef) Describe() string {
	if r.Namespace == "" {
		return r.Kind + " " + r.Name
	}
	return r.Kind + " " + r.Namespace + "/" + r.Name
}

// Ref returns the object the incident is about.
//
// It is the stored Object for any incident this build created. The decode
// below is the fallback for one restored from an older on-disk state, which
// records the display name and not the reference: it applies the naming
// conventions Incident.Name carries -- pod symptoms hold a bare owner name,
// workload objects hold "namespace/name", and nodes hold their name in
// NodeName with no namespace at all.
func (i *Incident) Ref() ObjectRef {
	if i == nil {
		return ObjectRef{}
	}
	if i.Object.Name != "" {
		return i.Object
	}
	return i.decodeRef()
}

// decodeRef recovers the subject reference from the display name. It is the
// one place that knows those conventions, and the only caller is Ref.
func (i *Incident) decodeRef() ObjectRef {
	if i.Resource == "node" {
		name := i.NodeName
		if name == "" {
			name = i.Name
		}
		return ObjectRef{Kind: "node", Name: name}
	}
	return NewObjectRef(i.Resource, i.Namespace, i.Name)
}

// SetObject freezes the incident's identity at creation. Callers set it once,
// after the display name is final.
func (i *Incident) SetObject() {
	i.Object = i.decodeRef()
}

// ObjectRefs returns every concrete Kubernetes object the incident is about.
//
// This is the counterpart to Ref: Ref answers "which workload is this about"
// (one logical subject, used for dedup and for finding the workload's node in
// the resource graph), while ObjectRefs answers "which objects would I look
// up to see whether this is still real". They differ for Pod incidents, which
// are keyed by their owning workload but are actually about a changing set of
// Pods, held in Resources.
//
// Both rules used to be re-derived by whoever needed them -- the graph key
// builder, the stale sweep -- each with its own idea of where the name lives.
// One of them looked up a Pod by its owning Deployment's name, which never
// matches anything. Keeping the encoding here means there is one place to be
// right.
func (i *Incident) ObjectRefs() []ObjectRef {
	if i == nil {
		return nil
	}
	if i.Resource == "pod" && len(i.Resources) > 0 {
		refs := make([]ObjectRef, 0, len(i.Resources))
		for name := range i.Resources {
			refs = append(refs, ObjectRef{
				Kind:      "pod",
				Namespace: i.Namespace,
				Name:      name,
			})
		}
		// Map order is random; sort so callers that stop early, or compare
		// keys, behave the same on every pass.
		sort.Slice(refs, func(a, b int) bool {
			return refs[a].Name < refs[b].Name
		})
		return refs
	}
	ref := i.Ref()
	if ref.Name == "" {
		return nil
	}
	return []ObjectRef{ref}
}
