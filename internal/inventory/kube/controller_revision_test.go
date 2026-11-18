package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/abahmed/kwatch/internal/inventory"
)

func revisionObject(t *testing.T) *unstructured.Unstructured {
	t.Helper()
	yes := true
	cr := &appsv1.ControllerRevision{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1",
			Kind: "ControllerRevision"},
		ObjectMeta: metav1.ObjectMeta{Namespace: "db", Name: "pg-6c9f",
			UID: "u1", OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "StatefulSet", Name: "pg",
				Controller: &yes}}},
		Data:     runtime.RawExtension{Raw: []byte(`{"spec":{}}`)},
		Revision: 3,
	}
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cr)
	require.NoError(t, err)
	return &unstructured.Unstructured{Object: content}
}

func TestControllerRevisionSchemaReadsRevisionAndOwner(t *testing.T) {
	desc, ok := ControllerRevisionSchema{}.Describe(revisionObject(t))
	require.True(t, ok)
	assert.Equal(t, inventory.CoreID(KindControllerRevision, "db",
		"pg-6c9f"), desc.ID)
	assert.Equal(t, "3", desc.Attributes[AttrRevision].AsText())
	assert.Equal(t, []inventory.EntityID{
		inventory.CoreID(KindStatefulSet, "db", "pg")},
		desc.Relations[inventory.OwnedBy])
}

func TestControllerRevisionSchemaReadsTypedObject(t *testing.T) {
	desc, ok := ControllerRevisionSchema{}.Describe(
		&appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{
			Namespace: "db", Name: "pg-1"}, Revision: 7})
	require.True(t, ok)
	assert.Equal(t, "7", desc.Attributes[AttrRevision].AsText())
}

func TestControllerRevisionSchemaRefusesOtherKinds(t *testing.T) {
	other := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1", "kind": "ControllerRevision",
		"metadata": map[string]any{"name": "x"}}}
	_, ok := ControllerRevisionSchema{}.Describe(other)
	assert.False(t, ok)
}

func TestRevisionTransformDropsTheSavedTemplate(t *testing.T) {
	out, err := revisionTransform(revisionObject(t))
	require.NoError(t, err)
	u := out.(*unstructured.Unstructured)
	_, hasData := u.Object["data"]
	assert.False(t, hasData)
	assert.EqualValues(t, 3, u.Object["revision"])
	assert.Equal(t, "pg-6c9f", u.GetName())
}

func TestControllerRevisionsWatchedWithTheirTransform(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1",
		Resource: "controllerrevisions"}
	p, ok := planResource(gvr, "ControllerRevision", false,
		map[Resource]bool{})
	require.True(t, ok)
	assert.Equal(t, WatchStatus, p.mode)
	_, isRevision := statusSchema(p).(ControllerRevisionSchema)
	assert.True(t, isRevision)
	out, err := transformFor(gvr)(revisionObject(t))
	require.NoError(t, err)
	_, hasData := out.(*unstructured.Unstructured).Object["data"]
	assert.False(t, hasData)
}
