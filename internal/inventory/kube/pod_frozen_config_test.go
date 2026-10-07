package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func frozenOf(t *testing.T, p *corev1.Pod) string {
	t.Helper()
	desc, ok := kube.PodSchema{}.Describe(p)
	assert.True(t, ok)
	return desc.Attributes[kube.AttrFrozenConfig].AsText()
}

func TestFrozenConfigCoversEnvAndSubPath(t *testing.T) {
	p := pod("p1")
	p.Spec.Volumes = []corev1.Volume{
		{Name: "cfg", VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "sub-config"}}}},
		{Name: "live", VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "live-config"}}}},
	}
	p.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{Name: "cfg", MountPath: "/etc/app.conf", SubPath: "app.conf"},
		{Name: "live", MountPath: "/etc/live"},
	}
	p.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "api-config"}}}}
	p.Spec.Containers[0].Env = []corev1.EnvVar{{
		Name: "TOKEN", ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "api-token"}, Key: "t"}}}}

	got := frozenOf(t, p)

	assert.Equal(t, "configmap/api-config,configmap/sub-config,"+
		"secret/api-token", got)
	assert.NotContains(t, got, "live-config")
}

func TestFrozenConfigIgnoresPlainVolumes(t *testing.T) {
	p := pod("p1")
	p.Spec.Volumes = []corev1.Volume{{Name: "live",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "live-config"}}}}}
	p.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{Name: "live", MountPath: "/etc/live"}}

	assert.Empty(t, frozenOf(t, p))
}
