package kube

import (
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/inventory"
)

// setPortAttributes records the container's declared ports and the
// ports its probes connect to, so a probe aimed at a port the container
// does not serve can be told from an application that fails its probe.
func setPortAttributes(attrs map[string]inventory.Value, c corev1.Container) {
	if declared := containerPorts(c); declared != "" {
		attrs[AttrContainerPorts] = inventory.Text(declared)
	}
	if probed := probePorts(c); probed != "" {
		attrs[AttrProbePorts] = inventory.Text(probed)
	}
}

func containerPorts(c corev1.Container) string {
	ports := make([]string, 0, len(c.Ports))
	for _, port := range c.Ports {
		ports = append(ports, strconv.Itoa(int(port.ContainerPort)))
	}
	return strings.Join(ports, ",")
}

// probePorts lists each probe's port once, in startup, readiness,
// liveness order. A named port is resolved to the container's port of
// that name; a name the container does not declare is kept as written.
func probePorts(c corev1.Container) string {
	var out []string
	seen := map[string]bool{}
	for _, probe := range []*corev1.Probe{
		c.StartupProbe, c.ReadinessProbe, c.LivenessProbe,
	} {
		port, ok := probePort(probe)
		if !ok {
			continue
		}
		value := resolvePort(c, port)
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return strings.Join(out, ",")
}

func probePort(probe *corev1.Probe) (intstr.IntOrString, bool) {
	switch {
	case probe == nil:
		return intstr.IntOrString{}, false
	case probe.HTTPGet != nil:
		return probe.HTTPGet.Port, true
	case probe.TCPSocket != nil:
		return probe.TCPSocket.Port, true
	case probe.GRPC != nil:
		return intstr.FromInt32(probe.GRPC.Port), true
	}
	return intstr.IntOrString{}, false
}

func resolvePort(c corev1.Container, port intstr.IntOrString) string {
	if port.Type == intstr.Int {
		return strconv.Itoa(port.IntValue())
	}
	for _, declared := range c.Ports {
		if declared.Name == port.StrVal {
			return strconv.Itoa(int(declared.ContainerPort))
		}
	}
	return port.StrVal
}
