package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func execProbe(command ...string) *corev1.Probe {
	return &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
		Exec: &corev1.ExecAction{Command: command}}}
}

// TestProbeCommandKeepsOnlyTheProgramName: an exec probe records the
// base name of the program it runs, never its arguments, even when
// they are written into the first element; other probes record none.
func TestProbeCommandKeepsOnlyTheProgramName(t *testing.T) {
	long := make([]byte, 100)
	for i := range long {
		long[i] = 'a'
	}
	cases := []struct {
		name    string
		command []string
		want    string
	}{
		{"path", []string{"/bin/grpc_health_probe", "-addr=:9090"},
			"grpc_health_probe"},
		{"shell", []string{"sh", "-c", "curl -H 'token: s3cret' x"}, "sh"},
		{"one element", []string{"/usr/bin/check --password=hunter2"},
			"check"},
		{"empty", nil, ""},
		{"blank", []string{"  "}, ""},
		{"bounded", []string{string(long)}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := pod("p1")
			p.Spec.Containers = []corev1.Container{{Name: "app",
				Image: "app:1", LivenessProbe: execProbe(tc.command...)}}
			attrs := containerDescriptionsViaSchema(t, p)[0].Attributes
			got := attrs[kube.ProbeAttr("liveness", "command")].AsText()
			assert.Equal(t, "exec", attrs[kube.ProbeAttr("liveness",
				"check")].AsText())
			if tc.name == "bounded" {
				assert.LessOrEqual(t, len(got), 64+len("…"))
				assert.NotEmpty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
			for _, v := range attrs {
				assert.NotContains(t, v.AsText(), "hunter2")
				assert.NotContains(t, v.AsText(), "s3cret")
			}
		})
	}
	p := pod("p2")
	p.Spec.Containers = []corev1.Container{{Name: "app", Image: "app:1",
		ReadinessProbe: httpProbe("/ready", intstr.FromInt32(8080))}}
	attrs := containerDescriptionsViaSchema(t, p)[0].Attributes
	_, set := attrs[kube.ProbeAttr("readiness", "command")]
	assert.False(t, set)
}
