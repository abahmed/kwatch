package resource

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
)

func TestPodRequestsUsesSchedulerSemantics(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			}}},
			InitContainers: []corev1.Container{{Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			}}},
			Overhead: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
	}

	cpu, memory := podRequests(pod)
	if cpu != 2100 {
		t.Fatalf("cpu request = %d milli, want 2100", cpu)
	}
	if memory != 1073741824+67108864 {
		t.Fatalf("memory request = %d bytes, want %d", memory, 1073741824+67108864)
	}
}
