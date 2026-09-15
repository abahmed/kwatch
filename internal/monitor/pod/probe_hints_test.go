package pod

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestBuildProbeHintLivenessHTTP(t *testing.T) {
	spec := &corev1.Container{
		Name: "app",
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(8080),
				},
			},
		},
	}
	hint := BuildProbeHint("LivenessProbeFailed", spec)
	assert.Contains(t, hint, "HTTP GET")
	assert.Contains(t, hint, "liveness")
}

func TestBuildProbeHintReadinessTCP(t *testing.T) {
	spec := &corev1.Container{
		Name: "app",
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(3306),
				},
			},
		},
	}
	hint := BuildProbeHint("ReadinessProbeFailed", spec)
	assert.Contains(t, hint, "TCP check")
	assert.Contains(t, hint, "readiness")
}

func TestBuildProbeHintStartupExec(t *testing.T) {
	spec := &corev1.Container{
		Name: "app",
		StartupProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/check", "--ready"},
				},
			},
		},
	}
	hint := BuildProbeHint("StartupProbeFailed", spec)
	assert.Contains(t, hint, "exec")
	assert.Contains(t, hint, "startup")
}

func TestBuildProbeHintNilProbe(t *testing.T) {
	spec := &corev1.Container{Name: "app"}
	hint := BuildProbeHint("LivenessProbeFailed", spec)
	assert.NotContains(t, hint, "(HTTP")
}

func TestProbeType(t *testing.T) {
	assert.Equal(t, "liveness", ProbeType("LivenessProbeFailed"))
	assert.Equal(t, "readiness", ProbeType("ReadinessProbeFailed"))
	assert.Equal(t, "startup", ProbeType("StartupProbeFailed"))
	assert.Equal(t, "probe", ProbeType("Unknown"))
	assert.Equal(t, "probe", ProbeType(""))
}
