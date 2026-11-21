package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// g2Mount plays a new pod stuck in ContainerCreating while the kubelet
// reports FailedMount with message every round.
func g2Mount(message string) func(*cluster, *workload) {
	return func(c *cluster, w *workload) {
		gapRounds(c, w, 6, 45*time.Second, func(int32) podState {
			return gapPending(waiting("ContainerCreating", ""))
		}, func(pod *corev1.Pod, round int32) {
			gapKubelet(c, pod, "FailedMount", message, round)
		})
	}
}

// g2Pull plays a new pod whose image pull fails with message.
func g2Pull(message string) func(*cluster, *workload) {
	return func(c *cluster, w *workload) {
		gapRounds(c, w, 6, 45*time.Second, func(round int32) podState {
			reason := "ErrImagePull"
			if round%2 == 0 {
				reason = "ImagePullBackOff"
			}
			return gapPending(waiting(reason, message))
		}, func(pod *corev1.Pod, round int32) {
			gapKubelet(c, pod, "Failed", message, round)
		})
	}
}
