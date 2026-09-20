package controlplane

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestDetectPodIssueFindsFailedContainer(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
			Name:      "api-server",
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "server",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
					Reason: "CrashLoopBackOff",
				}},
			}},
		},
	}

	observation := DetectPodIssue(pod)
	if observation == nil {
		t.Fatal("DetectPodIssue() returned no observation")
	}
	if observation.Reason != constant.ReasonControlPlaneComponentFailure {
		t.Fatalf("unexpected reason: %q", observation.Reason)
	}
	if observation.Container != "server" {
		t.Fatalf("unexpected container: %q", observation.Container)
	}
}

func TestComponentNameFromLabels(t *testing.T) {
	if got := ComponentNameFromLabels(map[string]string{
		"k8s-app": "kube-dns",
	}); got != "coredns" {
		t.Fatalf("ComponentNameFromLabels() = %q, want coredns", got)
	}
}
