package kube

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrHostPorts is on a pod whose containers ask for host ports: the
// node ports the scheduler must keep free for it. Each entry reads
// "TCP/0.0.0.0/8080", comma separated. A pod with none has no attribute.
const AttrHostPorts = "host.ports"

// maxHostPorts bounds the entries kept for one pod. It is far above
// what a real pod asks for; a pod past it is logged, not cut silently.
const maxHostPorts = 128

// wildcardIP is the address a host port without a hostIP binds.
const wildcardIP = "0.0.0.0"

// HostPort is a port a pod claims on its node.
type HostPort struct {
	Protocol string
	IP       string
	Port     int
}

// Conflicts reports whether the two claims cannot share a node: the
// same protocol and port, and addresses that overlap (the wildcard
// overlaps every address).
func (h HostPort) Conflicts(other HostPort) bool {
	if h.Protocol != other.Protocol || h.Port != other.Port {
		return false
	}
	return h.IP == other.IP || h.IP == wildcardIP || other.IP == wildcardIP
}

// String writes the claim as the scheduler's users say it: "TCP 8080",
// or "UDP 10.0.0.1:5353" for a claim on one address.
func (h HostPort) String() string {
	if h.IP == wildcardIP {
		return h.Protocol + " " + strconv.Itoa(h.Port)
	}
	return h.Protocol + " " + net.JoinHostPort(h.IP, strconv.Itoa(h.Port))
}

// ParseHostPorts decodes an AttrHostPorts value; an unreadable entry is
// skipped.
func ParseHostPorts(value string) []HostPort {
	var out []HostPort
	for _, entry := range strings.Split(value, ",") {
		parts := strings.Split(entry, "/")
		if len(parts) != 3 {
			continue
		}
		port, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}
		out = append(out,
			HostPort{Protocol: parts[0], IP: parts[1], Port: port})
	}
	return out
}

// hostPortEntry renders one container port as "protocol/ip/port",
// applying the Kubernetes defaults for an empty protocol or host IP.
func hostPortEntry(p corev1.ContainerPort) string {
	protocol, ip := string(p.Protocol), p.HostIP
	if protocol == "" {
		protocol = string(corev1.ProtocolTCP)
	}
	if ip == "" {
		ip = wildcardIP
	}
	return fmt.Sprintf("%s/%s/%d", protocol, ip, p.HostPort)
}

// setHostPorts records the host ports of the pod's containers, init
// and sidecar containers included: a sidecar keeps its port for the
// life of the pod.
func setHostPorts(attrs map[string]inventory.Value, pod *corev1.Pod) {
	var entries []string
	containers := append(append([]corev1.Container{},
		pod.Spec.InitContainers...), pod.Spec.Containers...)
	skipped := 0
	for _, c := range containers {
		for _, p := range c.Ports {
			if p.HostPort == 0 {
				continue
			}
			if len(entries) == maxHostPorts {
				skipped++
				continue
			}
			entries = append(entries, hostPortEntry(p))
		}
	}
	if skipped > 0 {
		klog.InfoS("host ports of a pod are capped", "pod",
			pod.Namespace+"/"+pod.Name, "kept", maxHostPorts,
			"skipped", skipped)
	}
	if len(entries) > 0 {
		attrs[AttrHostPorts] = inventory.Text(strings.Join(entries, ","))
	}
}
