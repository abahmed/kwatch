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
