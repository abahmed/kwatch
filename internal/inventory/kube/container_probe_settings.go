package kube

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// ProbeKinds are the three probes of a container, in the order a
// container runs them.
var ProbeKinds = []string{"startup", "readiness", "liveness"}

// Kubernetes' default for a probe timeout left at zero.
const defaultProbeTimeout = 1

// ProbeAttr names one setting attribute of one probe kind, such as
// ProbeAttr("readiness", "period") for "probe.readiness.period".
// Settings: "check", "handler" (the field that holds the check:
// "httpGet", "tcpSocket", "grpc", "exec"), "port", "delay", "period",
// "timeout", "failures".
func ProbeAttr(kind, setting string) string {
	return "probe." + kind + "." + setting
}

// setProbeSettings records, for each probe a container declares, what
// it checks and how it is timed, so a message can name the exact field
// to change. Fields left at zero hold Kubernetes' defaults.
func setProbeSettings(attrs map[string]inventory.Value, c corev1.Container) {
	for i, probe := range []*corev1.Probe{
		c.StartupProbe, c.ReadinessProbe, c.LivenessProbe,
	} {
		if probe == nil {
			continue
		}
		kind := ProbeKinds[i]
		if check := probeCheck(c, probe); check != "" {
			attrs[ProbeAttr(kind, "check")] = inventory.Text(check)
		}
		setProbeHandler(attrs, c, kind, probe)
		set := func(setting string, value, fallback int32) {
			if value <= 0 {
				value = fallback
			}
			attrs[ProbeAttr(kind, setting)] = inventory.Number(float64(value))
		}
		attrs[ProbeAttr(kind, "delay")] = inventory.Number(
			float64(probe.InitialDelaySeconds))
		set("period", probe.PeriodSeconds, defaultProbePeriod)
		set("timeout", probe.TimeoutSeconds, defaultProbeTimeout)
		set("failures", probe.FailureThreshold, defaultProbeFailures)
	}
}

// probeCheck says what a probe asks the container: "http /healthz
// :8080", "tcp :5432", "grpc :9090" or "exec".
func probeCheck(c corev1.Container, p *corev1.Probe) string {
	switch {
	case p.HTTPGet != nil:
		return "http " + p.HTTPGet.Path + " :" + resolvePort(c, p.HTTPGet.Port)
	case p.TCPSocket != nil:
		return "tcp :" + resolvePort(c, p.TCPSocket.Port)
	case p.GRPC != nil:
		return "grpc :" + strconv.Itoa(int(p.GRPC.Port))
	case p.Exec != nil:
		return "exec"
	}
	return ""
}

// setProbeHandler records the field that holds the probe's check and
// the port it connects to, when it has one.
func setProbeHandler(
	attrs map[string]inventory.Value, c corev1.Container, kind string,
	p *corev1.Probe,
) {
	handler, port := "", ""
	switch {
	case p.HTTPGet != nil:
		handler, port = "httpGet", resolvePort(c, p.HTTPGet.Port)
	case p.TCPSocket != nil:
		handler, port = "tcpSocket", resolvePort(c, p.TCPSocket.Port)
	case p.GRPC != nil:
		handler, port = "grpc", strconv.Itoa(int(p.GRPC.Port))
	case p.Exec != nil:
		handler = "exec"
	}
	if handler != "" {
		attrs[ProbeAttr(kind, "handler")] = inventory.Text(handler)
	}
	if port != "" {
		attrs[ProbeAttr(kind, "port")] = inventory.Text(port)
	}
}
