package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func optionalRefsPod() *corev1.Pod {
	yes, no := true, false
	local := func(name string) corev1.LocalObjectReference {
		return corev1.LocalObjectReference{Name: name}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "a", VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "opt-vol", Optional: &yes,
					}}},
				{Name: "b", VolumeSource: corev1.VolumeSource{
					Projected: &corev1.ProjectedVolumeSource{
						Sources: []corev1.VolumeProjection{{
							ConfigMap: &corev1.ConfigMapProjection{
								LocalObjectReference: local("opt-proj"),
								Optional:             &yes,
							}}}}}},
			},
			Containers: []corev1.Container{{
				Name: "app",
				EnvFrom: []corev1.EnvFromSource{{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: local("shared"),
						Optional:             &yes,
					}}},
				Env: []corev1.EnvVar{{
					Name: "X", ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: local("shared"),
							Key:                  "k", Optional: &no,
						}}}},
			}},
		},
	}
}

func TestPodSchemaMarksOptionalReferences(t *testing.T) {
	desc, ok := kube.PodSchema{}.Describe(optionalRefsPod())
	require.True(t, ok)

	refs := desc.Relations[inventory.References]
	assert.Contains(t, refs, inventory.CoreID(kube.KindSecret, "ns",
		"opt-vol"), "optional objects stay related for change tracking")
	assert.Equal(t, "configmap/opt-proj,secret/opt-vol",
		desc.Attributes[kube.AttrOptionalRefs].AsText())
}

func TestOptionalReferenceRequiresEveryUseOptional(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	tr := kube.NewTranslator(kube.PodSchema{})
	for _, f := range tr.Added(optionalRefsPod(), true, time.Unix(0, 0)) {
		_, err := model.Apply(f)
		require.NoError(t, err)
	}
	pod, ok := model.Entity(inventory.CoreID(kube.KindPod, "ns", "p"))
	require.True(t, ok)

	assert.True(t, kube.OptionalReference(pod,
		inventory.CoreID(kube.KindSecret, "ns", "opt-vol")))
	assert.False(t, kube.OptionalReference(pod,
		inventory.CoreID(kube.KindSecret, "ns", "shared")),
		"a required use makes the object required")
	assert.False(t, kube.OptionalReference(pod,
		inventory.CoreID(kube.KindConfigMap, "ns", "opt-vol")))
}

// The kubelet pulls without a pull Secret it cannot read, so a Secret
// named only in imagePullSecrets is one the pod starts without. A use
// in the environment or a volume keeps it required.
func TestPullSecretOnlyReferenceIsOptional(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: corev1.PodSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "regcred"}, {Name: "both"}},
			Containers: []corev1.Container{{
				Name: "app", Image: "app:1",
				EnvFrom: []corev1.EnvFromSource{{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "both"}}}},
			}},
		},
	}
	desc, ok := kube.PodSchema{}.Describe(pod)
	require.True(t, ok)

	assert.Contains(t, desc.Relations[inventory.References],
		inventory.CoreID(kube.KindSecret, "ns", "regcred"),
		"the pull Secret stays related for change tracking")
	assert.Equal(t, "secret/regcred",
		desc.Attributes[kube.AttrOptionalRefs].AsText())
}
