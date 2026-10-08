package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Pod states a scenario applies to a pod.

// podState edits a pod into one runtime state.
type podState func(c *cluster, pod *corev1.Pod)

// pod builds replica i of the workload's current ReplicaSet on node, in
// the running and ready state unless states say otherwise.
func (w *workload) pod(i int, node string, states ...podState) *corev1.Pod {
	rs := w.replicaSet
	yes := true
	name := rs.Name + "-" + string(rune('a'+i%26)) + itoa(i/26)
	started := metav1.NewTime(w.c.start.Add(-time.Hour))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: rs.Namespace, UID: types.UID(name),
			Labels:            rs.Spec.Template.Labels,
			CreationTimestamp: started,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "ReplicaSet", Name: rs.Name,
				UID: rs.UID, Controller: &yes,
			}},
		},
		Spec: *rs.Spec.Template.Spec.DeepCopy(),
	}
	if node != "" {
		pod.Spec.NodeName = w.c.n(node)
	}
	running(w.c, pod)
	for _, state := range states {
		state(w.c, pod)
	}
	return pod
}

func itoa(i int) string {
	if i == 0 {
		return ""
	}
	digits := ""
	for ; i > 0; i /= 10 {
		digits = string(rune('0'+i%10)) + digits
	}
	return digits
}

// running is a started, ready pod.
func running(c *cluster, pod *corev1.Pod) {
	since := metav1.NewTime(c.start.Add(-time.Hour))
	pod.Status = corev1.PodStatus{
		Phase: corev1.PodRunning, StartTime: &since,
		QOSClass: corev1.PodQOSBurstable,
		Conditions: []corev1.PodCondition{
			{Type: corev1.PodScheduled, Status: corev1.ConditionTrue,
				LastTransitionTime: since},
			{Type: corev1.PodReady, Status: corev1.ConditionTrue,
				LastTransitionTime: since},
			{Type: corev1.ContainersReady, Status: corev1.ConditionTrue,
				LastTransitionTime: since},
		},
	}
	for _, container := range pod.Spec.Containers {
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses,
			corev1.ContainerStatus{
				Name: container.Name, Image: container.Image, Ready: true,
				Started: boolPtr(true),
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{StartedAt: since},
				},
			})
	}
}

// startedNow marks the pod as started at the current time, as a fresh
// replica of a rollout is.
func startedNow(c *cluster, pod *corev1.Pod) {
	now := metav1.NewTime(c.now)
	pod.CreationTimestamp = now
	pod.Status.StartTime = &now
	for i := range pod.Status.Conditions {
		pod.Status.Conditions[i].LastTransitionTime = now
	}
	for i := range pod.Status.ContainerStatuses {
		if r := pod.Status.ContainerStatuses[i].State.Running; r != nil {
			r.StartedAt = now
		}
	}
}

// createdAt dates the pod's creation and start at, as a replica a
// rollout created then is, while later states move the clock on.
func createdAt(at time.Time) podState {
	return func(_ *cluster, pod *corev1.Pod) {
		created := metav1.NewTime(at)
		pod.CreationTimestamp = created
		pod.Status.StartTime = &created
	}
}

// notReady turns the pod's readiness off since the current time.
func notReady(c *cluster, pod *corev1.Pod) {
	now := metav1.NewTime(c.now)
	for i := range pod.Status.Conditions {
		switch pod.Status.Conditions[i].Type {
		case corev1.PodReady, corev1.ContainersReady:
			pod.Status.Conditions[i].Status = corev1.ConditionFalse
			pod.Status.Conditions[i].Reason = "ContainersNotReady"
			pod.Status.Conditions[i].LastTransitionTime = now
		}
	}
	for i := range pod.Status.ContainerStatuses {
		pod.Status.ContainerStatuses[i].Ready = false
	}
}

// notReadySince turns the pod's readiness off since at.
func notReadySince(at time.Time) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		for i := range pod.Status.Conditions {
			switch pod.Status.Conditions[i].Type {
			case corev1.PodReady, corev1.ContainersReady:
				pod.Status.Conditions[i].LastTransitionTime =
					metav1.NewTime(at)
			}
		}
	}
}

// crashLoop is a container that keeps exiting with code and message.
func crashLoop(
	exitCode int32, reason, message string, restarts int32,
) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.Started = boolPtr(false)
		status.RestartCount = restarts
		status.State = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  "CrashLoopBackOff",
				Message: "back-off 5m0s restarting failed container",
			},
		}
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: exitCode, Reason: reason, Message: message,
				StartedAt:  metav1.NewTime(c.now.Add(-20 * time.Second)),
				FinishedAt: metav1.NewTime(c.now.Add(-10 * time.Second)),
			},
		}
	}
}

// waiting is a container that cannot start, such as ImagePullBackOff or
// CreateContainerConfigError.
func waiting(reason, message string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.Started = boolPtr(false)
		status.State = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason: reason, Message: message,
			},
		}
	}
}

// pendingUnscheduled is a pod the scheduler cannot place.
func pendingUnscheduled(message string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		now := metav1.NewTime(c.now)
		pod.Spec.NodeName = ""
		pod.CreationTimestamp = now
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodPending, QOSClass: corev1.PodQOSBurstable,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse,
				Reason: "Unschedulable", Message: message,
				LastTransitionTime: now,
			}},
		}
	}
}

// evicted is a pod the kubelet evicted.
func evicted(message string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		pod.Status.Phase = corev1.PodFailed
		pod.Status.Reason = "Evicted"
		pod.Status.Message = message
		for i := range pod.Status.ContainerStatuses {
			pod.Status.ContainerStatuses[i].State = corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					ExitCode: 137, Reason: "ContainerStatusUnknown",
					FinishedAt: metav1.NewTime(c.now),
				},
			}
		}
	}
}
