package kube

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/inventory"
)

// KindControllerRevision is the entity kind of a ControllerRevision: one
// saved pod template of a StatefulSet or DaemonSet, numbered by the
// revision "kubectl rollout undo --to-revision" names.
const KindControllerRevision inventory.Kind = "controllerrevision"

// LabelRevisionHash is the label a StatefulSet or DaemonSet pod carries
// to name the ControllerRevision it was made from: the revision's name
// for a StatefulSet, the hash suffix of that name for a DaemonSet.
const LabelRevisionHash = appsv1.ControllerRevisionHashLabelKey

// controllerRevisionResource is watched with its revision number kept,
// not metadata only: the number is a top-level field, and a rollback
// to a known revision needs it. The rest of the object (the saved
// template) is dropped from the cache (revisionTransform).
var controllerRevisionResource = Resource{
	Group: "apps", Name: "controllerrevisions",
}

func isControllerRevision(gvr schema.GroupVersionResource) bool {
	return gvr.Group == controllerRevisionResource.Group &&
		gvr.Resource == controllerRevisionResource.Name
}

// revisionTransform keeps a ControllerRevision's metadata and revision
// number and drops the saved template.
func revisionTransform(obj any) (any, error) {
	obj, err := TrimToLatestManager(obj)
	if err != nil {
		return obj, err
	}
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return obj, nil
	}
	out := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": u.GetAPIVersion(), "kind": u.GetKind(),
	}}
	if metadata, ok := u.Object["metadata"].(map[string]any); ok {
		out.Object["metadata"] = metadata
	}
	dropLastApplied(out)
	if revision, ok := u.Object["revision"]; ok {
		out.Object["revision"] = revision
	}
	return out, nil
}

// ControllerRevisionSchema describes ControllerRevisions: the workload
// that owns each and its revision number (AttrRevision).
type ControllerRevisionSchema struct{}

// Kind implements Schema.
func (ControllerRevisionSchema) Kind() inventory.Kind {
	return KindControllerRevision
}

// RelationTypes implements Schema.
func (ControllerRevisionSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.OwnedBy}
}

// Describe implements Schema. It reads the typed object and the trimmed
// unstructured one the dynamic watch caches.
func (ControllerRevisionSchema) Describe(obj any) (Description, bool) {
	meta, revision, ok := revisionParts(obj)
	if !ok {
		return Description{}, false
	}
	attrs := KindNameAttributes("ControllerRevision")
	if revision > 0 {
		attrs[AttrRevision] = inventory.Text(
			strconv.FormatInt(revision, 10))
	}
	rel := relations{}
	rel.add(inventory.OwnedBy, ownerIDs(meta)...)
	return Description{
		ID: inventory.CoreID(KindControllerRevision,
			meta.GetNamespace(), meta.GetName()),
		UID: string(meta.GetUID()), Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema. A revision is immutable: its template and
// number never change.
func (ControllerRevisionSchema) Diff(_, _ any) []inventory.FieldChange {
	return nil
}

// revisionParts reads the metadata and revision number of a typed or
// unstructured ControllerRevision.
func revisionParts(obj any) (metav1.Object, int64, bool) {
	switch typed := obj.(type) {
	case *appsv1.ControllerRevision:
		return typed, typed.Revision, true
	case *unstructured.Unstructured:
		if typed.GetKind() != "ControllerRevision" ||
			GroupForAPIVersion(typed.GetAPIVersion()) != "" {
			return nil, 0, false
		}
		revision, _, _ := unstructured.NestedInt64(typed.Object, "revision")
		return typed, revision, true
	}
	return nil, 0, false
}

// PodRevisionHash is the controller-revision-hash label of a pod, or ""
// when it has none.
func PodRevisionHash(r inventory.Reader, pod inventory.EntityID) string {
	a, ok := r.Attribute(pod, AttrLabels)
	if !ok {
		return ""
	}
	set, err := labels.ConvertSelectorToLabelsMap(a.Value.AsText())
	if err != nil {
		return ""
	}
	return set[LabelRevisionHash]
}
