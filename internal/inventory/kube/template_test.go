package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestTemplateHash(t *testing.T) {
	tt := []struct {
		name     string
		template *corev1.PodTemplateSpec
		checkFn  func(*testing.T, string)
	}{
		{
			name:     "nil_template",
			template: nil,
			checkFn: func(t *testing.T, hash string) {
				assert.Equal(t, "", hash)
			},
		},
		{
			name:     "empty_template",
			template: &corev1.PodTemplateSpec{},
			checkFn: func(t *testing.T, hash string) {
				assert.NotEmpty(t, hash)
			},
		},
		{
			name: "same_templates_same_hash",
			template: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:1.0"},
					},
				},
			},
			checkFn: func(t *testing.T, hash1 string) {
				template2 := &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "app:1.0"},
						},
					},
				}
				hash2 := kube.TemplateHash(template2)
				assert.Equal(t, hash1, hash2)
			},
		},
		{
			name: "different_images_different_hash",
			template: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:1.0"},
					},
				},
			},
			checkFn: func(t *testing.T, hash1 string) {
				template2 := &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "app:2.0"},
						},
					},
				}
				hash2 := kube.TemplateHash(template2)
				assert.NotEqual(t, hash1, hash2)
			},
		},
		{
			name: "same_spec_same_hash",
			template: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Image: "app:1.0"},
					},
				},
			},
			checkFn: func(t *testing.T, hash1 string) {
				template2 := &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "app:1.0"},
						},
					},
				}
				hash2 := kube.TemplateHash(template2)
				// Hash should be the same
				assert.Equal(t, hash1, hash2)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			hash := kube.TemplateHash(tc.template)
			tc.checkFn(t, hash)
		})
	}
}

func TestDeploymentSchemaDiff(t *testing.T) {
	tt := []struct {
		name    string
		old     *appsv1.Deployment
		new     *appsv1.Deployment
		checkFn func(*testing.T, []inventory.FieldChange)
	}{
		{
			name: "image_rollout",
			old:  deployment("d1"),
			new: func() *appsv1.Deployment {
				d := deployment("d1")
				d.Spec.Template.Spec.Containers[0].Image =
					"app:2.0"
				return d
			}(),
			checkFn: func(t *testing.T, changes []inventory.FieldChange) {
				found := false
				for _, c := range changes {
					if c.Path ==
						"containers[app].image" {
						found = true
						assert.Equal(t, "app:1.0",
							c.Before)
						assert.Equal(t, "app:2.0",
							c.After)
					}
				}
				assert.True(t, found,
					"image change not found")
			},
		},
		{
			name: "replicas_change",
			old:  deployment("d1"),
			new: func() *appsv1.Deployment {
				d := deployment("d1")
				replicas := int32(5)
				d.Spec.Replicas = &replicas
				return d
			}(),
			checkFn: func(t *testing.T, changes []inventory.FieldChange) {
				found := false
				for _, c := range changes {
					if c.Path == "spec.replicas" {
						found = true
						assert.Equal(t, "3",
							c.Before)
						assert.Equal(t, "5",
							c.After)
					}
				}
				assert.True(t, found,
					"replicas change not found")
			},
		},
		{
			name: "paused_changed",
			old:  deployment("d1"),
			new: func() *appsv1.Deployment {
				d := deployment("d1")
				d.Spec.Paused = true
				return d
			}(),
			checkFn: func(t *testing.T, changes []inventory.FieldChange) {
				found := false
				for _, c := range changes {
					if c.Path == "spec.paused" {
						found = true
						assert.Equal(t, "false",
							c.Before)
						assert.Equal(t, "true",
							c.After)
					}
				}
				assert.True(t, found,
					"paused change not found")
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			schema := kube.DeploymentSchema()
			changes := schema.Diff(tc.old, tc.new)
			if tc.checkFn != nil {
				tc.checkFn(t, changes)
			}
		})
	}
}

func TestReplicaSetSchemaDiff(t *testing.T) {
	old := replicaSet("rs1")
	replicas := int32(5)
	new := replicaSet("rs1")
	new.Spec.Replicas = &replicas

	schema := kube.ReplicaSetSchema()
	changes := schema.Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Path == "spec.replicas" {
			found = true
			assert.Equal(t, "2", c.Before)
			assert.Equal(t, "5", c.After)
		}
	}
	assert.True(t, found, "replicas change not found")
}
