package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// readyJobPod gives a Job's pod the Ready conditions a kubelet reports
// for a running container, so a pod that later fails or is deleted was
// healthy until then.
func readyJobPod(c *cluster, pod *corev1.Pod) *corev1.Pod {
	since := metav1.NewTime(c.now)
	for _, kind := range []corev1.PodConditionType{
		corev1.PodScheduled, corev1.ContainersReady, corev1.PodReady} {
		pod.Status.Conditions = append(pod.Status.Conditions,
			corev1.PodCondition{Type: kind, Status: corev1.ConditionTrue,
				LastTransitionTime: since})
	}
	for i := range pod.Status.ContainerStatuses {
		pod.Status.ContainerStatuses[i].Ready = true
	}
	return pod
}

// evictedSince is an eviction that finished at the given time: the pod
// lingers in the list long after the incident that evicted it.
func evictedSince(at time.Time, message string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		evicted(message)(c, pod)
		when := metav1.NewTime(at)
		for i := range pod.Status.Conditions {
			switch pod.Status.Conditions[i].Type {
			case corev1.PodReady, corev1.ContainersReady:
				pod.Status.Conditions[i].LastTransitionTime = when
			}
		}
		for i := range pod.Status.ContainerStatuses {
			if t := pod.Status.ContainerStatuses[i].State.Terminated; t != nil {
				t.FinishedAt = when
			}
		}
	}
}
