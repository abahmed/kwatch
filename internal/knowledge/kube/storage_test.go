package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func TestPVCSchemaDescribe(t *testing.T) {
	p := pvc("pvc1")
	schema := kube.PVCSchema{}
	desc, ok := schema.Describe(p)

	assert.True(t, ok)
	assert.Equal(t, knowledge.Kind("persistentvolumeclaim"),
		desc.ID.Kind)
	assert.Equal(t, "pvc1", desc.ID.Name)

	// Check phase
	phase, ok := desc.Attributes["phase"]
	assert.True(t, ok)
	assert.Equal(t, "Bound", phase.AsText())

	// Check access modes
	modes, ok := desc.Attributes["access.modes"]
	assert.True(t, ok)
	assert.Contains(t, modes.AsText(), "ReadWriteOnce")

	// Check requested storage
	requested, ok := desc.Attributes["storage.requested"]
	assert.True(t, ok)
	num, _ := requested.AsNumber()
	assert.Greater(t, num, 0.0)
}

func TestPVCReferencesVolume(t *testing.T) {
	p := pvc("pvc1")
	p.Spec.VolumeName = "pv1"

	schema := kube.PVCSchema{}
	desc, ok := schema.Describe(p)
	assert.True(t, ok)

	refs := desc.Relations[knowledge.References]
	assert.GreaterOrEqual(t, len(refs), 1)
	found := false
	for _, r := range refs {
		if r.Kind == knowledge.Kind("persistentvolume") &&
			r.Name == "pv1" {
			found = true
		}
	}
	assert.True(t, found, "PV reference not found")
}

func TestPVCReferencesStorageClass(t *testing.T) {
	p := pvc("pvc1")
	scName := "fast-ssd"
	p.Spec.StorageClassName = &scName

	schema := kube.PVCSchema{}
	desc, ok := schema.Describe(p)
	assert.True(t, ok)

	refs := desc.Relations[knowledge.References]
	found := false
	for _, r := range refs {
		if r.Kind == knowledge.Kind("storageclass") &&
			r.Name == "fast-ssd" {
			found = true
		}
	}
	assert.True(t, found, "StorageClass reference not found")
}

func TestPVCResizeDiff(t *testing.T) {
	old := pvc("pvc1")
	new := pvc("pvc1")
	new.Spec.Resources.Requests[corev1.ResourceStorage] =
		mustQuantity("20Gi")

	schema := kube.PVCSchema{}
	changes := schema.Diff(old, new)

	assert.Len(t, changes, 1)
	assert.Equal(t, "spec.resources.requests.storage",
		changes[0].Path)
	assert.Equal(t, "10Gi", changes[0].Before)
	assert.Equal(t, "20Gi", changes[0].After)
}

func TestPVSchemaDescribe(t *testing.T) {
	p := pv("pv1")
	schema := kube.PVSchema{}
	desc, ok := schema.Describe(p)

	assert.True(t, ok)
	assert.Equal(t, knowledge.Kind("persistentvolume"),
		desc.ID.Kind)
	assert.Equal(t, "pv1", desc.ID.Name)
	// PVs are cluster-scoped
	assert.Equal(t, "", desc.ID.Namespace)

	// Check phase
	phase, ok := desc.Attributes["phase"]
	assert.True(t, ok)
	assert.Equal(t, "Bound", phase.AsText())

	// Check capacity
	capacity, ok := desc.Attributes["storage.capacity"]
	assert.True(t, ok)
	num, _ := capacity.AsNumber()
	assert.Greater(t, num, 0.0)
}

func TestPVReferencesStorageClass(t *testing.T) {
	p := pv("pv1")
	p.Spec.StorageClassName = "fast"

	schema := kube.PVSchema{}
	desc, ok := schema.Describe(p)
	assert.True(t, ok)

	refs := desc.Relations[knowledge.References]
	assert.Len(t, refs, 1)
	assert.Equal(t, knowledge.Kind("storageclass"),
		refs[0].Kind)
	assert.Equal(t, "fast", refs[0].Name)
}

func TestPVNoDiff(t *testing.T) {
	p1 := pv("pv1")
	p2 := pv("pv1")

	schema := kube.PVSchema{}
	changes := schema.Diff(p1, p2)

	// PV diffs return nil
	assert.Nil(t, changes)
}

func TestStorageClassSchemaDescribe(t *testing.T) {
	sc := storageClass("sc1")
	schema := kube.StorageClassSchema{}
	desc, ok := schema.Describe(sc)

	assert.True(t, ok)
	assert.Equal(t, knowledge.Kind("storageclass"),
		desc.ID.Kind)
	assert.Equal(t, "sc1", desc.ID.Name)
	// StorageClasses are cluster-scoped
	assert.Equal(t, "", desc.ID.Namespace)

	// Check provisioner
	provisioner, ok := desc.Attributes["provisioner"]
	assert.True(t, ok)
	assert.Equal(t, "ebs.csi.aws.com", provisioner.AsText())
}

func TestStorageClassNoDiff(t *testing.T) {
	sc1 := storageClass("sc1")
	sc2 := storageClass("sc1")

	schema := kube.StorageClassSchema{}
	changes := schema.Diff(sc1, sc2)

	// StorageClass diffs return nil
	assert.Nil(t, changes)
}

func TestNamespaceSchemaDescribe(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testns",
		},
		Status: corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		},
	}

	schema := kube.NamespaceSchema{}
	desc, ok := schema.Describe(ns)

	assert.True(t, ok)
	assert.Equal(t, knowledge.Kind("namespace"), desc.ID.Kind)
	assert.Equal(t, "testns", desc.ID.Name)
	// Namespaces are cluster-scoped
	assert.Equal(t, "", desc.ID.Namespace)

	// Check phase
	phase, ok := desc.Attributes["phase"]
	assert.True(t, ok)
	assert.Equal(t, "Active", phase.AsText())
}

func TestNamespaceConditions(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testns",
		},
		Status: corev1.NamespaceStatus{
			Phase: corev1.NamespaceActive,
		},
	}

	schema := kube.NamespaceSchema{}
	desc, ok := schema.Describe(ns)
	assert.True(t, ok)

	// Even though there are no conditions in the
	// namespace, the describe should work
	_, ok = desc.Attributes["phase"]
	assert.True(t, ok)
}

func TestNamespaceNoDiff(t *testing.T) {
	ns1 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testns",
		},
	}
	ns2 := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testns",
		},
	}

	schema := kube.NamespaceSchema{}
	changes := schema.Diff(ns1, ns2)

	// Namespace diffs return nil
	assert.Nil(t, changes)
}

func TestPVCWithCapacity(t *testing.T) {
	p := pvc("pvc1")
	p.Status.Capacity = corev1.ResourceList{
		corev1.ResourceStorage: mustQuantity("10Gi"),
	}

	schema := kube.PVCSchema{}
	desc, ok := schema.Describe(p)
	assert.True(t, ok)

	// Check that capacity is recorded
	capacity, ok := desc.Attributes["storage.capacity"]
	assert.True(t, ok)
	num, _ := capacity.AsNumber()
	assert.Greater(t, num, 0.0)
}

func TestAccessModesText(t *testing.T) {
	tt := []struct {
		name  string
		modes []corev1.PersistentVolumeAccessMode
		want  string
	}{
		{
			name: "single_mode",
			modes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			want: "ReadWriteOnce",
		},
		{
			name: "multiple_modes",
			modes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
				corev1.ReadOnlyMany,
			},
			want: "ReadWriteOnce,ReadOnlyMany",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			p := pvc("pvc1")
			p.Spec.AccessModes = tc.modes

			schema := kube.PVCSchema{}
			desc, ok := schema.Describe(p)
			assert.True(t, ok)

			modes, ok := desc.Attributes["access.modes"]
			assert.True(t, ok)
			assert.Contains(t, modes.AsText(), "ReadWrite")
		})
	}
}
