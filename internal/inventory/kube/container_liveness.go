package kube

import (
	"slices"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attributes of a container's liveness probe.
const (
	// AttrLivenessDelay, AttrLivenessPeriod and AttrLivenessFailures
	// are the probe's initial delay and period in seconds and the
	// failures it allows. They are written only for a container with a
	// liveness probe and no startup probe: a startup probe holds the
	// liveness probe back until the container has started.
	AttrLivenessDelay    = "liveness.delay.seconds"
	AttrLivenessPeriod   = "liveness.period.seconds"
	AttrLivenessFailures = "liveness.failures"
	// AttrLivenessSameCheck is true when the liveness probe runs the
	// same check as the readiness probe: the same HTTP path and port,
	// the same TCP port, the same command or the same gRPC service.
	AttrLivenessSameCheck = "liveness.same.check"
)

// Kubernetes' defaults for a probe field left at zero.
const (
	defaultProbePeriod   = 10
	defaultProbeFailures = 3
)

// setLivenessAttributes records how long the liveness probe allows a
// container to start, and whether it checks what readiness checks.
func setLivenessAttributes(
	attrs map[string]inventory.Value, c corev1.Container,
) {
	live := c.LivenessProbe
	if live == nil {
		return
	}
	if c.ReadinessProbe != nil && sameCheck(c, live, c.ReadinessProbe) {
		attrs[AttrLivenessSameCheck] = inventory.Bool(true)
	}
	if c.StartupProbe != nil {
		return
	}
	period, failures := live.PeriodSeconds, live.FailureThreshold
	if period <= 0 {
		period = defaultProbePeriod
	}
	if failures <= 0 {
		failures = defaultProbeFailures
	}
	attrs[AttrLivenessDelay] = inventory.Number(
		float64(live.InitialDelaySeconds))
	attrs[AttrLivenessPeriod] = inventory.Number(float64(period))
	attrs[AttrLivenessFailures] = inventory.Number(float64(failures))
}

// sameCheck reports two probes that ask the container the same thing.
// Timing is not part of the check; a named port counts as its number.
func sameCheck(c corev1.Container, a, b *corev1.Probe) bool {
	switch {
	case a.HTTPGet != nil && b.HTTPGet != nil:
		return a.HTTPGet.Path == b.HTTPGet.Path &&
			resolvePort(c, a.HTTPGet.Port) == resolvePort(c, b.HTTPGet.Port) &&
			a.HTTPGet.Scheme == b.HTTPGet.Scheme
	case a.TCPSocket != nil && b.TCPSocket != nil:
		return resolvePort(c, a.TCPSocket.Port) ==
			resolvePort(c, b.TCPSocket.Port)
	case a.Exec != nil && b.Exec != nil:
		return slices.Equal(a.Exec.Command, b.Exec.Command)
	case a.GRPC != nil && b.GRPC != nil:
		return a.GRPC.Port == b.GRPC.Port &&
			ptrText(a.GRPC.Service) == ptrText(b.GRPC.Service)
	}
	return false
}

func ptrText(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
