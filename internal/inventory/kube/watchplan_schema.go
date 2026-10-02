package kube

import (
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
)

// genericSchema adds the generic attributes (generic_attributes.go) to
// every description of the schema it wraps. Values the wrapped schema
// set are kept.
type genericSchema struct {
	Schema
	mode WatchMode
}

// withGenericAttributes wraps s so its entities carry the generic
// attributes and the watch mode.
func withGenericAttributes(s Schema, mode WatchMode) Schema {
	return genericSchema{Schema: s, mode: mode}
}

// Describe implements Schema.
func (s genericSchema) Describe(obj any) (Description, bool) {
	desc, ok := s.Schema.Describe(obj)
	if !ok {
		return desc, false
	}
	if desc.Attributes == nil {
		desc.Attributes = make(map[string]inventory.Value)
	}
	attrs := desc.Attributes
	if accessor, err := meta.Accessor(obj); err == nil {
		setMetadataAttributes(attrs, accessor)
	}
	if u, ok := obj.(*unstructured.Unstructured); ok {
		setStatusAttributes(attrs, u)
	}
	attrs[AttrWatchMode] = inventory.Text(string(s.mode))
	return desc, true
}

// setMetadataAttributes sets generation, deletion and finalizers.
func setMetadataAttributes(
	attrs map[string]inventory.Value, obj metav1.Object,
) {
	setDefault(attrs, AttrGeneration,
		inventory.Number(float64(obj.GetGeneration())))
	deleting := obj.GetDeletionTimestamp()
	setDefault(attrs, AttrDeleting, inventory.Bool(deleting != nil))
	if deleting != nil {
		setDefault(attrs, AttrDeletingSince, inventory.Time(deleting.Time))
	}
	if finalizers := obj.GetFinalizers(); len(finalizers) > 0 {
		setDefault(attrs, AttrFinalizers,
			inventory.Text(evidenceText(strings.Join(finalizers, ","))))
	}
}

// setStatusAttributes sets the phase and observed generation a status
// cache carries.
func setStatusAttributes(
	attrs map[string]inventory.Value, u *unstructured.Unstructured,
) {
	if phase, ok, _ := unstructured.NestedString(
		u.Object, "status", "phase"); ok && phase != "" {
		setDefault(attrs, AttrPhase, inventory.Text(evidenceText(phase)))
	}
	if observed, ok, _ := unstructured.NestedInt64(
		u.Object, "status", "observedGeneration"); ok {
		setDefault(attrs, AttrObservedGen, inventory.Number(float64(observed)))
	}
}

func setDefault(
	attrs map[string]inventory.Value, key string, value inventory.Value,
) {
	if _, ok := attrs[key]; !ok {
		attrs[key] = value
	}
}

// MetadataSchema describes metadata-only objects of one kind: identity,
// owners, generation, deletion and finalizers. Metadata informers do not
// report an object's kind, so the schema is bound to one.
type MetadataSchema struct {
	group string
	kind  inventory.Kind
}

// NewMetadataSchema builds a schema for one Kubernetes Kind of one API
// group ("" for the core group).
func NewMetadataSchema(apiGroup, kubernetesKind string) MetadataSchema {
	return MetadataSchema{
		group: GroupFor(apiGroup), kind: KindFor(kubernetesKind),
	}
}

// Kind implements Schema.
func (s MetadataSchema) Kind() inventory.Kind { return s.kind }

// RelationTypes implements Schema.
func (MetadataSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.OwnedBy}
}

// Describe implements Schema.
func (s MetadataSchema) Describe(obj any) (Description, bool) {
	m, ok := obj.(*metav1.PartialObjectMetadata)
	if !ok {
		return Description{}, false
	}
	rel := relations{}
	rel.add(inventory.OwnedBy, ownerIDs(m)...)
	return Description{
		ID: inventory.NewEntityID(
			s.group, s.kind, m.GetNamespace(), m.GetName()),
		UID:        string(m.GetUID()),
		Attributes: map[string]inventory.Value{},
		Relations:  rel,
	}, true
}

// Diff implements Schema: a generation increase is a spec change.
func (MetadataSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*metav1.PartialObjectMetadata)
	after, ok2 := new.(*metav1.PartialObjectMetadata)
	if !ok1 || !ok2 || after.Generation == 0 ||
		before.Generation == after.Generation {
		return nil
	}
	return []inventory.FieldChange{{
		Path:   "spec",
		Before: "generation " + strconv.FormatInt(before.Generation, 10),
		After:  "generation " + strconv.FormatInt(after.Generation, 10),
	}}
}
