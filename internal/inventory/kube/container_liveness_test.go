package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func httpProbe(path string, port intstr.IntOrString) *corev1.Probe {
	return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{Path: path, Port: port}}}
}

// TestLivenessAttributes: the liveness timing is recorded, with the
// Kubernetes defaults for fields left at zero, and the probe is marked
// when it runs the same check as readiness.
func TestLivenessAttributes(t *testing.T) {
	p := pod("p1")
	live := httpProbe("/healthz", intstr.FromString("http"))
	live.InitialDelaySeconds = 10
	p.Spec.Containers = []corev1.Container{{
		Name: "app", Image: "app:1",
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: 8080}},
		LivenessProbe:  live,
		ReadinessProbe: httpProbe("/healthz", intstr.FromInt32(8080)),
	}}
	attrs := containerDescriptionsViaSchema(t, p)[0].Attributes
	num := func(name string) float64 {
		n, _ := attrs[name].AsNumber()
		return n
	}
	assert.Equal(t, 10.0, num(kube.AttrLivenessDelay))
	assert.Equal(t, 10.0, num(kube.AttrLivenessPeriod))
	assert.Equal(t, 3.0, num(kube.AttrLivenessFailures))
	same, _ := attrs[kube.AttrLivenessSameCheck].AsBool()
	assert.True(t, same)
}

// TestLivenessAttributesNeedNoStartupProbe: a startup probe holds
// liveness back, so no start budget is recorded; a different path is
// not the same check.
func TestLivenessAttributesNeedNoStartupProbe(t *testing.T) {
	p := pod("p1")
	p.Spec.Containers = []corev1.Container{{
		Name: "app", Image: "app:1",
		LivenessProbe:  httpProbe("/live", intstr.FromInt32(8080)),
		ReadinessProbe: httpProbe("/ready", intstr.FromInt32(8080)),
		StartupProbe:   httpProbe("/live", intstr.FromInt32(8080)),
	}}
	attrs := containerDescriptionsViaSchema(t, p)[0].Attributes
	require.NotContains(t, attrs, kube.AttrLivenessDelay)
	require.NotContains(t, attrs, kube.AttrLivenessSameCheck)
}
