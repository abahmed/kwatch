package kube

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Description is everything a Schema extracts from one object version.
type Description struct {
	ID         knowledge.EntityID
	UID        string
	Attributes map[string]knowledge.Value
	Relations  map[knowledge.RelationType][]knowledge.EntityID
	// Children are dependent entities described with the object, such as a
	// pod's containers. They share the object's lifecycle.
	Children []Description
}

// Schema knows one Kubernetes kind.
type Schema interface {
	// Kind is the entity kind this schema produces.
	Kind() knowledge.Kind
	// RelationTypes lists every relation type Describe can produce, so the
	// translator can clear a type an object no longer has even when the
	// previous version was never seen.
	RelationTypes() []knowledge.RelationType
	// Describe extracts identity, attributes and relations. It returns
	// false for objects that are not of this schema's type.
	Describe(obj any) (Description, bool)
	// Diff lists meaningful changes between two versions. Status-only
	// updates return nil.
	Diff(old, new any) []knowledge.FieldChange
}

// relations is a small builder that skips empty targets.
type relations map[knowledge.RelationType][]knowledge.EntityID

func (r relations) add(
	rel knowledge.RelationType, targets ...knowledge.EntityID,
) {
	for _, target := range targets {
		if target.Name != "" {
			r[rel] = append(r[rel], target)
		}
	}
}

func objectID(kind knowledge.Kind, meta metav1.Object) knowledge.EntityID {
	return knowledge.NewEntityID(kind, meta.GetNamespace(), meta.GetName())
}

// ownerIDs converts controller owner references into owned-by targets.
func ownerIDs(meta metav1.Object) []knowledge.EntityID {
	var out []knowledge.EntityID
	for _, ref := range meta.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			out = append(out, knowledge.NewEntityID(
				KindFor(ref.Kind), meta.GetNamespace(), ref.Name,
			))
		}
	}
	return out
}
