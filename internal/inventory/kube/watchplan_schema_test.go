package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory"
)

var deletedAt = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func deletingMeta(name string) metav1.ObjectMeta {
	at := metav1.NewTime(deletedAt)
	return metav1.ObjectMeta{
		Name: name, Namespace: "ns", Generation: 4,
		DeletionTimestamp: &at, Finalizers: []string{"a", "b"},
	}
}

func describeWith(
	t *testing.T, s Schema, mode WatchMode, obj any,
) Description {
	t.Helper()
	desc, ok := withGenericAttributes(s, mode).Describe(obj)
	require.True(t, ok)
	return desc
}

func TestGenericAttributesOnTypedKinds(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: deletingMeta("p"),
		Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	attrs := describeWith(t, PodSchema{}, WatchFull, pod).Attributes

	assert.Equal(t, inventory.Text("full"), attrs[AttrWatchMode])
	assert.Equal(t, inventory.Bool(true), attrs[AttrDeleting])
	assert.Equal(t, inventory.Time(deletedAt), attrs[AttrDeletingSince])
	assert.Equal(t, inventory.Text("a,b"), attrs[AttrFinalizers])
	assert.Equal(t, inventory.Number(4), attrs[AttrGeneration])
	assert.Equal(t, inventory.Text("Running"), attrs[AttrPhase])

	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "c"}}
	attrs = describeWith(t, ConfigMapSchema{}, WatchHashed, cm).Attributes
	assert.Equal(t, inventory.Text("hashed"), attrs[AttrWatchMode])
	assert.Equal(t, inventory.Bool(false), attrs[AttrDeleting])
	assert.NotContains(t, attrs, AttrDeletingSince)
	assert.NotContains(t, attrs, AttrFinalizers)
}

func TestGenericAttributesInStatusMode(t *testing.T) {
	w := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1", "kind": "Widget",
		"metadata": map[string]any{
			"name": "w", "namespace": "ns", "generation": int64(3),
			"finalizers": []any{"example.com/cleanup"},
		},
		"status": map[string]any{
			"phase": "Provisioning", "observedGeneration": int64(2),
			"conditions": []any{map[string]any{
				"type": "Ready", "status": "False", "reason": "Waiting",
			}},
		},
	}}
	desc := describeWith(t,
		NewUnstructuredSchema("example.com", "Widget"), WatchStatus, w)
	attrs := desc.Attributes

	assert.Equal(t, inventory.Text("status"), attrs[AttrWatchMode])
	assert.Equal(t, inventory.Number(3), attrs[AttrGeneration])
	assert.Equal(t, inventory.Number(2), attrs[AttrObservedGen])
	assert.Equal(t, inventory.Text("Provisioning"), attrs[AttrPhase])
	assert.Equal(t, inventory.Text("False"), attrs[ConditionKey("Ready")])
	assert.Equal(t, inventory.Text("example.com/cleanup"),
		attrs[AttrFinalizers])
	assert.Equal(t, inventory.Bool(false), attrs[AttrDeleting])
}

func TestGenericAttributesInMetadataMode(t *testing.T) {
	controller := true
	meta := deletingMeta("l")
	meta.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1", Kind: "Deployment", Name: "d",
		Controller: &controller,
	}}
	lease := &metav1.PartialObjectMetadata{ObjectMeta: meta}
	desc := describeWith(t,
		NewMetadataSchema("coordination.k8s.io", "Lease"),
		WatchMetadata, lease)

	assert.Equal(t, inventory.NewEntityID("", "lease", "ns", "l"), desc.ID)
	assert.Equal(t, map[string]inventory.Value{
		AttrWatchMode:     inventory.Text("metadata"),
		AttrGeneration:    inventory.Number(4),
		AttrDeleting:      inventory.Bool(true),
		AttrDeletingSince: inventory.Time(deletedAt),
		AttrFinalizers:    inventory.Text("a,b"),
	}, desc.Attributes)
	assert.Equal(t,
		[]inventory.EntityID{inventory.CoreID("deployment", "ns", "d")},
		desc.Relations[inventory.OwnedBy])
}

func TestMetadataSchemaRejectsOtherObjects(t *testing.T) {
	_, ok := NewMetadataSchema("", "Lease").Describe(&corev1.Pod{})
	assert.False(t, ok)
	_, ok = withGenericAttributes(
		NewMetadataSchema("", "Lease"), WatchMetadata).Describe("x")
	assert.False(t, ok)
}

func TestMetadataSchemaDiffsOnGeneration(t *testing.T) {
	s := NewMetadataSchema("apps", "ControllerRevision")
	v := func(generation int64) *metav1.PartialObjectMetadata {
		return &metav1.PartialObjectMetadata{
			ObjectMeta: metav1.ObjectMeta{Generation: generation},
		}
	}
	assert.Nil(t, s.Diff(v(1), v(1)))
	assert.Nil(t, s.Diff(v(1), v(0)))
	assert.Nil(t, s.Diff("x", v(2)))
	changes := s.Diff(v(1), v(2))
	require.Len(t, changes, 1)
	assert.Equal(t, "generation 2", changes[0].After)
	assert.Equal(t, []inventory.RelationType{inventory.OwnedBy},
		s.RelationTypes())
	assert.Equal(t, inventory.Kind("controllerrevision"), s.Kind())
}
