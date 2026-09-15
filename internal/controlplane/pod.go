package controlplane

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

var controlPlaneSelectors = map[string]string{
	"kube-apiserver":          "component=kube-apiserver",
	"kube-scheduler":          "component=kube-scheduler",
	"kube-controller-manager": "component=kube-controller-manager",
	"etcd":                    "component=etcd",
	"kube-proxy":              "k8s-app=kube-proxy",
	"coredns":                 "k8s-app=kube-dns",
	"metrics-server":          "k8s-app=metrics-server",
}

// DetectPodIssue evaluates one control-plane Pod without external state.
func DetectPodIssue(pod *corev1.Pod) *model.Observation {
	if pod == nil || pod.Status.Phase == corev1.PodSucceeded {
		return nil
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady &&
			condition.Status == corev1.ConditionFalse {
			if condition.Reason == constant.ReasonPodCompleted {
				continue
			}
			break
		}
	}

	statuses := make([]corev1.ContainerStatus, 0,
		len(pod.Status.ContainerStatuses)+len(pod.Status.InitContainerStatuses))
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	for _, status := range statuses {
		if observation := containerIssue(pod, status); observation != nil {
			return observation
		}
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled &&
			condition.Status == corev1.ConditionFalse {
			return controlPlaneObservation(pod).WithHint(fmt.Sprintf(
				"control-plane component %s/%s: %s: %s",
				pod.Namespace, pod.Name, condition.Reason, condition.Message,
			))
		}
	}
	return nil
}

func containerIssue(
	pod *corev1.Pod, status corev1.ContainerStatus,
) *model.Observation {
	reason := containerReason(status)
	if reason == "" {
		return nil
	}
	observation := controlPlaneObservation(pod)
	observation.Container = status.Name
	observation.Image = status.Image
	observation.RestartCount = status.RestartCount
	return observation.WithHint(fmt.Sprintf(
		"control-plane component %s/%s: container %s has issue: %s",
		pod.Namespace, pod.Name, status.Name, reason,
	))
}

func controlPlaneObservation(pod *corev1.Pod) *model.Observation {
	return observe.ObjectNamed(
		"controlplane", pod.Namespace, pod.Name,
		constant.ReasonControlPlaneComponentFailure,
	).WithLabels(pod.Labels).WithSeverity(model.SeverityHigh)
}

func containerReason(status corev1.ContainerStatus) string {
	if waiting := status.State.Waiting; waiting != nil {
		if waiting.Reason == constant.ReasonContainerCreating ||
			waiting.Reason == constant.ReasonPodInitializing {
			return ""
		}
		if waiting.Reason == constant.ReasonCrashLoopBackOff &&
			status.LastTerminationState.Terminated != nil {
			return status.LastTerminationState.Terminated.Reason
		}
		return waiting.Reason
	}
	if terminated := status.State.Terminated; terminated != nil {
		if terminated.ExitCode == 0 ||
			terminated.Reason == constant.ReasonCompleted {
			return ""
		}
		return terminated.Reason
	}
	return ""
}

// ComponentNameFromLabels identifies a known control-plane component.
func ComponentNameFromLabels(labels map[string]string) string {
	for name, selector := range controlPlaneSelectors {
		for key, value := range labels {
			if selector == key+"="+value {
				return name
			}
		}
	}
	return ""
}
