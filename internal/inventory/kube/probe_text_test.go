package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// A probe that leaves period, timeout or threshold unset runs with the
// Kubernetes defaults, and the text says so instead of "every 0s".
func TestProbeTextUsesDefaultsForUnsetFields(t *testing.T) {
	probe := &corev1.Probe{ProbeHandler: corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{Path: "/healthz",
			Port: intstr.FromInt(8080)}}}
	assert.Equal(t, "http /healthz :8080 every 10s, timeout 1s, 3 failures",
		probeText(probe))
	probe.PeriodSeconds, probe.TimeoutSeconds = 5, 2
	probe.FailureThreshold = 6
	assert.Equal(t, "http /healthz :8080 every 5s, timeout 2s, 6 failures",
		probeText(probe))
}
