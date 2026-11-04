package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestLastFailureLinePicksTheNewestFailure(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{"last error wins", []string{"error: first", "fatal: second",
			"info: ok"}, "fatal: second"},
		{"unhealthy words", []string{"component cache is unhealthy",
			"debug: tick"}, "component cache is unhealthy"},
		{"not ready", []string{"store not ready yet"},
			"store not ready yet"},
		{"answered status", []string{"GET /ready status=503"},
			"GET /ready status=503"},
		{"stack frames skipped", []string{"error: pool exhausted",
			"at com.example.Pool.take(Pool.java:42)"},
			"error: pool exhausted"},
		{"nothing failing", []string{"info: started", "uptime 512 s"},
			""},
		{"no lines", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, kube.LastFailureLine(tc.lines))
		})
	}
}

func TestProbeAnsweredStatusReadsOnlyAnsweredProbes(t *testing.T) {
	assert.Equal(t, 503, kube.ProbeAnsweredStatus(
		"Liveness probe failed: HTTP probe failed with statuscode: 503"))
	assert.Zero(t, kube.ProbeAnsweredStatus(refusedProbe))
	assert.Zero(t, kube.ProbeAnsweredStatus(""))
}

func TestProbeLogObservationKeepsOnlyAFailureLine(t *testing.T) {
	id := kube.ContainerID(testNamespace, "p", "app")
	quiet := kube.ProbeLogObservation(id, fixedTime(),
		[]string{"info: serving"})
	assert.Empty(t, quiet.Attributes)
	loud := kube.ProbeLogObservation(id, fixedTime(),
		[]string{healthLine})
	assert.Equal(t, healthLine,
		loud.Attributes[kube.AttrProbeLogLine].AsText())
}
