package kube

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Description is everything a Schema extracts from one object version.
type Description struct {
	ID  inventory.EntityID
	UID string
	// AltUID is a second identifier events name the object by.
	AltUID     string
	Attributes map[string]inventory.Value
	Relations  map[inventory.RelationType][]inventory.EntityID
	// Children are dependent entities described with the object, such as a
	// pod's containers. They share the object's lifecycle.
	Children []Description
}

// Schema knows one Kubernetes kind.
type Schema interface {
	// Kind is the entity kind this schema produces.
	Kind() inventory.Kind
	// RelationTypes lists every relation type Describe can produce, so the
	// translator can clear a type an object no longer has even when the
	// previous version was never seen.
	RelationTypes() []inventory.RelationType
	// Describe extracts identity, attributes and relations. It returns
	// false for objects that are not of this schema's type.
	Describe(obj any) (Description, bool)
	// Diff lists meaningful changes between two versions. Status-only
	// updates return nil.
	Diff(old, new any) []inventory.FieldChange
}

// relations is a small builder that skips empty targets.
type relations map[inventory.RelationType][]inventory.EntityID

func (r relations) add(
	rel inventory.RelationType, targets ...inventory.EntityID,
) {
	for _, target := range targets {
		if target.Name != "" {
			r[rel] = append(r[rel], target)
		}
	}
}

func objectID(kind inventory.Kind, meta metav1.Object) inventory.EntityID {
	return inventory.CoreID(kind, meta.GetNamespace(), meta.GetName())
}

// ownerIDs converts controller owner references into owned-by targets.
// The owner's apiVersion sets its group, so an owner whose kind is also a
// built-in kind (a Knative Service) never resolves to the built-in one.
func ownerIDs(meta metav1.Object) []inventory.EntityID {
	var out []inventory.EntityID
	for _, ref := range meta.GetOwnerReferences() {
		if ref.Controller != nil && *ref.Controller {
			out = append(out, EntityFor(
				ref.APIVersion, ref.Kind, meta.GetNamespace(), ref.Name,
			))
		}
	}
	return out
}
