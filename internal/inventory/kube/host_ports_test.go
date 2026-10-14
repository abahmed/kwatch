package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func hostPortPod(ports ...corev1.ContainerPort) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "app", Ports: ports}}}}
}

func TestHostPortsAbsentWithoutHostPort(t *testing.T) {
	pod := hostPortPod(corev1.ContainerPort{ContainerPort: 80})
	_, has := describePod(t, pod).Attributes[AttrHostPorts]
	assert.False(t, has)
}

func TestHostPortsRoundTrip(t *testing.T) {
	pod := hostPortPod(
		corev1.ContainerPort{ContainerPort: 80, HostPort: 8080},
		corev1.ContainerPort{ContainerPort: 53, HostPort: 5353,
			Protocol: corev1.ProtocolUDP, HostIP: "10.0.0.1"})
	d := describePod(t, pod)
	got := ParseHostPorts(d.Attributes[AttrHostPorts].AsText())
	assert.Equal(t, []HostPort{
		{Protocol: "TCP", IP: "0.0.0.0", Port: 8080},
		{Protocol: "UDP", IP: "10.0.0.1", Port: 5353},
	}, got)
	assert.Equal(t, "TCP 8080", got[0].String())
	assert.Equal(t, "UDP 10.0.0.1:5353", got[1].String())
}

func TestHostPortConflicts(t *testing.T) {
	tcp := func(ip string, port int) HostPort {
		return HostPort{Protocol: "TCP", IP: ip, Port: port}
	}
	assert.True(t, tcp("0.0.0.0", 80).Conflicts(tcp("0.0.0.0", 80)))
	assert.True(t, tcp("0.0.0.0", 80).Conflicts(tcp("10.0.0.1", 80)))
	assert.True(t, tcp("10.0.0.1", 80).Conflicts(tcp("0.0.0.0", 80)))
	assert.False(t, tcp("10.0.0.1", 80).Conflicts(tcp("10.0.0.2", 80)))
	assert.False(t, tcp("0.0.0.0", 80).Conflicts(tcp("0.0.0.0", 81)))
	udp := HostPort{Protocol: "UDP", IP: "0.0.0.0", Port: 80}
	assert.False(t, tcp("0.0.0.0", 80).Conflicts(udp))
}

func TestHostPortStringBracketsIPv6(t *testing.T) {
	h := HostPort{Protocol: "TCP", IP: "::1", Port: 80}
	assert.Equal(t, "TCP [::1]:80", h.String())
}

func TestHostPortsIncludeInitAndSidecarContainers(t *testing.T) {
	always := corev1.ContainerRestartPolicyAlways
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "side",
				RestartPolicy: &always, Ports: []corev1.ContainerPort{
					{ContainerPort: 1, HostPort: 9000}}}},
			Containers: []corev1.Container{{Name: "app",
				Ports: []corev1.ContainerPort{
					{ContainerPort: 2, HostPort: 9001}}}}}}
	got := ParseHostPorts(describePod(t, pod).
		Attributes[AttrHostPorts].AsText())
	assert.Equal(t, []HostPort{
		{Protocol: "TCP", IP: "0.0.0.0", Port: 9000},
		{Protocol: "TCP", IP: "0.0.0.0", Port: 9001}}, got)
}

// Many host ports are all kept up to the bound, which is far above
// what a real pod asks for.
func TestHostPortsKeepWhatARealPodAsksFor(t *testing.T) {
	var ports []corev1.ContainerPort
	for i := 0; i < 100; i++ {
		ports = append(ports,
			corev1.ContainerPort{ContainerPort: 1, HostPort: int32(9000 + i)})
	}
	got := ParseHostPorts(describePod(t, hostPortPod(ports...)).
		Attributes[AttrHostPorts].AsText())
	assert.Len(t, got, 100)
}
