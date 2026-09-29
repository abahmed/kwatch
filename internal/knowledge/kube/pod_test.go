package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func TestPodSchemaDescribe(t *testing.T) {
	tt := []struct {
		name    string
		obj     any
		wantOK  bool
		checkFn func(*testing.T, kube.Description)
	}{
		{
			name:   "valid_pod",
			obj:    pod("p1"),
			wantOK: true,
			checkFn: func(t *testing.T, desc kube.Description) {
				assert.Equal(t, knowledge.Kind("pod"),
					desc.ID.Kind)
				assert.Equal(t, "p1", desc.ID.Name)
				assert.Equal(t, testNamespace,
					desc.ID.Namespace)
				// Check attributes
				phase, ok := desc.Attributes["phase"]
				assert.True(t, ok)
				assert.Equal(t, "Running", phase.AsText())
				// Check ready attribute
				ready, ok := desc.Attributes["ready"]
				assert.True(t, ok)
				b, _ := ready.AsBool()
				assert.True(t, b)
			},
		},
		{
			name:   "pod_with_deletion",
			obj:    pod("p1"),
			wantOK: true,
			checkFn: func(t *testing.T, desc kube.Description) {
				// Check deletion attribute when set
				del, ok := desc.Attributes["deleting"]
				assert.True(t, ok)
				b, _ := del.AsBool()
				assert.False(t, b)
			},
		},
		{
			name:   "pod_with_children",
			obj:    pod("p1"),
			wantOK: true,
			checkFn: func(t *testing.T, desc kube.Description) {
				// Pod should have containers as children
				assert.Len(t, desc.Children, 1)
				child := desc.Children[0]
				assert.Equal(t,
					knowledge.Kind("container"),
					child.ID.Kind)
			},
		},
		{
			name:   "wrong_type",
			obj:    "not a pod",
			wantOK: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			schema := kube.PodSchema{}
			desc, ok := schema.Describe(tc.obj)
			assert.Equal(t, tc.wantOK, ok)
			if ok && tc.checkFn != nil {
				tc.checkFn(t, desc)
			}
		})
	}
}

func TestPodSchemaDiff(t *testing.T) {
	tt := []struct {
		name    string
		old     any
		new     any
		wantLen int
		checkFn func(*testing.T, []knowledge.FieldChange)
	}{
		{
			name:    "no_change",
			old:     pod("p1"),
			new:     pod("p1"),
			wantLen: 0,
		},
		{
			name: "deletion_timestamp_set",
			old:  pod("p1"),
			new: func() any {
				p := pod("p1")
				t := metav1.NewTime(fixedTime())
				p.DeletionTimestamp = &t
				return p
			}(),
			wantLen: 1,
			checkFn: func(t *testing.T, changes []knowledge.FieldChange) {
				assert.Equal(t,
					"metadata.deletionTimestamp",
					changes[0].Path)
				assert.Equal(t, "set",
					changes[0].After)
			},
		},
		{
			name: "resource_resize_memory_limit",
			old:  pod("p1"),
			new: func() any {
				p := pod("p1")
				if p.Spec.Containers[0].Resources.Limits == nil {
					p.Spec.Containers[0].Resources.Limits =
						corev1.ResourceList{}
				}
				q := mustQuantity("512Mi")
				p.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory] = q
				return p
			}(),
			wantLen: 1, // One change: memory limit
			checkFn: func(t *testing.T,
				changes []knowledge.FieldChange) {
				// Should have memory limit change
				found := false
				for _, c := range changes {
					if c.Path ==
						"containers[app].resources."+
							"limits.memory" {
						found = true
						assert.Equal(t, "", c.Before)
						assert.Equal(t, "512Mi",
							c.After)
					}
				}
				assert.True(t, found,
					"memory limit change not found")
			},
		},
		{
			name:    "wrong_types",
			old:     "not a pod",
			new:     "also not a pod",
			wantLen: 0,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			schema := kube.PodSchema{}
			changes := schema.Diff(tc.old, tc.new)
			assert.Len(t, changes, tc.wantLen)
			if tc.checkFn != nil {
				tc.checkFn(t, changes)
			}
		})
	}
}

func TestPodRelationTypes(t *testing.T) {
	schema := kube.PodSchema{}
	types := schema.RelationTypes()
	assert.Contains(t, types, knowledge.OwnedBy)
	assert.Contains(t, types, knowledge.RunsOn)
	assert.Contains(t, types, knowledge.References)
	assert.Contains(t, types, knowledge.Mounts)
	assert.Contains(t, types, knowledge.Pulls)
}

func TestPodReferences(t *testing.T) {
	tt := []struct {
		name    string
		pod     *corev1.Pod
		checkFn func(*testing.T, kube.Description)
	}{
		{
			name: "references_secret",
			pod: func() *corev1.Pod {
				p := pod("p1")
				p.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
					{Name: "pull-secret"},
				}
				return p
			}(),
			checkFn: func(t *testing.T, desc kube.Description) {
				refs := desc.Relations[knowledge.References]
				found := false
				for _, r := range refs {
					if r.Kind == knowledge.Kind("secret") &&
						r.Name == "pull-secret" {
						found = true
					}
				}
				assert.True(t, found,
					"pull secret reference not found")
			},
		},
		{
			name: "references_service_account",
			pod: func() *corev1.Pod {
				p := pod("p1")
				p.Spec.ServiceAccountName = "mysa"
				return p
			}(),
			checkFn: func(t *testing.T, desc kube.Description) {
				refs := desc.Relations[knowledge.References]
				found := false
				for _, r := range refs {
					if r.Kind == knowledge.Kind(
						"serviceaccount") &&
						r.Name == "mysa" {
						found = true
					}
				}
				assert.True(t, found,
					"service account reference not found")
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			schema := kube.PodSchema{}
			desc, ok := schema.Describe(tc.pod)
			assert.True(t, ok)
			tc.checkFn(t, desc)
		})
	}
}

func TestPodMountsVolumes(t *testing.T) {
	p := pod("p1")
	p.Spec.Volumes = []corev1.Volume{
		{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.
					PersistentVolumeClaimVolumeSource{
					ClaimName: "my-pvc",
				},
			},
		},
	}
	schema := kube.PodSchema{}
	desc, ok := schema.Describe(p)
	assert.True(t, ok)
	mounts := desc.Relations[knowledge.Mounts]
	assert.Len(t, mounts, 1)
	assert.Equal(t, knowledge.Kind("persistentvolumeclaim"),
		mounts[0].Kind)
	assert.Equal(t, "my-pvc", mounts[0].Name)
}

func TestPodPullsImages(t *testing.T) {
	p := pod("p1")
	p.Spec.Containers = []corev1.Container{
		{Name: "app", Image: "app:1.0"},
	}
	schema := kube.PodSchema{}
	desc, ok := schema.Describe(p)
	assert.True(t, ok)
	pulls := desc.Relations[knowledge.Pulls]
	assert.GreaterOrEqual(t, len(pulls), 1)
	found := false
	for _, pull := range pulls {
		if pull.Kind == knowledge.Kind("image") &&
			pull.Name == "app:1.0" {
			found = true
		}
	}
	assert.True(t, found, "app:1.0 image not found")
}
