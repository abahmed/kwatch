package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestDriverNamesKeepsOnlyDriverFlagsAndVariables(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "c",
		Args: []string{"--driver-name=Ebs.Example.com", "--token=secret",
			"--provisioner", "b.example.com", "--v=5"},
		Env: []corev1.EnvVar{
			{Name: "DRIVER_NAME", Value: "c.example.com"},
			{Name: "API_KEY", Value: "secret"},
			{Name: "DRIVER_TOKEN", ValueFrom: &corev1.EnvVarSource{}},
		},
	}}}}

	got := driverNames(pod)

	if got != "ebs.example.com b.example.com c.example.com" {
		t.Fatalf("got %q", got)
	}
}

// A node-driver-registrar is given the driver's name only inside the
// kubelet socket paths: the registration path argument, the socket
// variable and the plugin directory it mounts. The name is kept, never
// the rest of the path.
func TestDriverNamesReadsTheNameFromKubeletPluginPaths(t *testing.T) {
	const dir = "/var/lib/kubelet/plugins/"
	pod := &corev1.Pod{Spec: corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "registrar",
			Args: []string{"--csi-address=/csi/csi.sock",
				"--kubelet-registration-path=" + dir +
					"a.example.com/csi.sock"},
			Env: []corev1.EnvVar{
				{Name: "DRIVER_REG_SOCK_PATH",
					Value: dir + "b.example.com/csi.sock"},
				{Name: "ADDRESS", Value: "/csi/csi.sock"},
			},
		}},
		Volumes: []corev1.Volume{
			{Name: "plugin-dir", VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: dir + "c.example.com/"}}},
			{Name: "registration", VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/kubelet/plugins_registry/"}}},
			{Name: "kubelet", VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: dir + "kubernetes.io/csi"}}},
		},
	}}

	got := driverNames(pod)

	if got != "a.example.com b.example.com c.example.com" {
		t.Fatalf("got %q", got)
	}
}

// The registration socket file is named after the driver.
func TestDriverNamesReadsTheRegistrationSocketName(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "registrar",
		Env: []corev1.EnvVar{{Name: "KUBELET_REGISTRATION_SOCKET",
			Value: "/var/lib/kubelet/plugins_registry/" +
				"d.example.com-reg.sock"}},
	}}}}

	if got := driverNames(pod); got != "d.example.com" {
		t.Fatalf("got %q", got)
	}
}
