package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// TestProbeSettingsAreRecordedPerProbe: each declared probe keeps what
// it checks and how it is timed, with Kubernetes' defaults for fields
// left at zero; a probe that is not declared leaves nothing.
func TestProbeSettingsAreRecordedPerProbe(t *testing.T) {
	p := pod("p1")
	ready := httpProbe("/ready", intstr.FromString("http"))
	ready.InitialDelaySeconds = 5
	ready.TimeoutSeconds = 2
	p.Spec.Containers = []corev1.Container{{
		Name: "app", Image: "app:1",
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8080}},
		ReadinessProbe: ready,
		LivenessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt32(5432)}}},
	}}
	attrs := containerDescriptionsViaSchema(t, p)[0].Attributes
	num := func(kind, setting string) float64 {
		n, _ := attrs[kube.ProbeAttr(kind, setting)].AsNumber()
		return n
	}
	text := func(kind, setting string) string {
		return attrs[kube.ProbeAttr(kind, setting)].AsText()
	}

	assert.Equal(t, "http /ready :8080", text("readiness", "check"))
	assert.Equal(t, "httpGet", text("readiness", "handler"))
	assert.Equal(t, "8080", text("readiness", "port"))
	assert.Equal(t, 5.0, num("readiness", "delay"))
	assert.Equal(t, 10.0, num("readiness", "period"))
	assert.Equal(t, 2.0, num("readiness", "timeout"))
	assert.Equal(t, 3.0, num("readiness", "failures"))
	assert.Equal(t, "tcpSocket", text("liveness", "handler"))
	assert.Equal(t, "5432", text("liveness", "port"))
	assert.Equal(t, 1.0, num("liveness", "timeout"))
	_, declared := attrs[kube.ProbeAttr("startup", "period")]
	assert.False(t, declared)
}

// TestRefAttributesNameTheFieldThatRefersToAnObject: the field path is
// recorded under the object's own name, and never a value.
func TestRefAttributesNameTheFieldThatRefersToAnObject(t *testing.T) {
	p := pod("p1")
	p.Spec.Volumes = []corev1.Volume{{Name: "certs",
		VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
			SecretName: "tls"}}}}
	p.Spec.Containers = []corev1.Container{{
		Name: "app", Image: "app:1",
		EnvFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.
			ConfigMapEnvSource{LocalObjectReference: corev1.
			LocalObjectReference{Name: "settings"}}}},
		Env: []corev1.EnvVar{{Name: "STRIPE_KEY", Value: "",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.
				SecretKeySelector{LocalObjectReference: corev1.
				LocalObjectReference{Name: "stripe-api"}, Key: "key"}}},
			{Name: "PLAIN", Value: "hunter2"}},
	}}
	desc, ok := kube.PodSchema{}.Describe(p)
	assert.True(t, ok)
	container := desc.Children[0].Attributes

	assert.Equal(t, "env[STRIPE_KEY].valueFrom.secretKeyRef",
		container[kube.RefAttr(kube.KindSecret, "stripe-api")].AsText())
	assert.Equal(t, "envFrom.configMapRef",
		container[kube.RefAttr(kube.KindConfigMap, "settings")].AsText())
	assert.Equal(t, "volumes[certs].secret",
		desc.Attributes[kube.RefAttr(kube.KindSecret, "tls")].AsText())
	for _, value := range container {
		assert.NotContains(t, value.AsText(), "hunter2")
	}
}

// TestBudgetKeepsItsOwnSetting: a disruption budget records the field
// it sets, as written.
func TestBudgetKeepsItsOwnSetting(t *testing.T) {
	min := intstr.FromInt32(2)
	pdb := &policyv1.PodDisruptionBudget{Spec: policyv1.
		PodDisruptionBudgetSpec{MinAvailable: &min}}
	pdb.Name, pdb.Namespace = "web", "shop"
	d, _ := kube.PDBSchema{}.Describe(pdb)

	assert.Equal(t, "2", d.Attributes[kube.AttrMinAvailable].AsText())
	_, set := d.Attributes[kube.AttrMaxUnavailable]
	assert.False(t, set)

	pct := intstr.FromString("50%")
	pdb.Spec = policyv1.PodDisruptionBudgetSpec{MaxUnavailable: &pct}
	d, _ = kube.PDBSchema{}.Describe(pdb)
	assert.Equal(t, "50%", d.Attributes[kube.AttrMaxUnavailable].AsText())
}
