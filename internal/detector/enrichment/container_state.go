package enrichment

import (
	"time"

	"github.com/abahmed/kwatch/internal/detector"
	corev1 "k8s.io/api/core/v1"
)

type ContainerStateEnricher struct{}

func NewContainerStateEnricher() *ContainerStateEnricher {
	return &ContainerStateEnricher{}
}

func (e *ContainerStateEnricher) Name() string {
	return "ContainerStateEnricher"
}

func (e *ContainerStateEnricher) Enrich(input *detector.Input) error {
	if input.Pod == nil {
		return nil
	}

	for _, containerStatus := range input.Pod.Status.ContainerStatuses {
		if containerStatus.LastTerminationState.Terminated != nil {
			terminated := containerStatus.LastTerminationState.Terminated
			if input.Container != nil && containerStatus.Name == input.Container.Name {
				input.ExitCode = terminated.ExitCode
				input.Reason = terminated.Reason
				input.Message = terminated.Message
				if !terminated.FinishedAt.IsZero() {
					input.LastTerminatedOn = terminated.FinishedAt.Time
				}
				break
			}
		}
	}

	return nil
}

func ExtractContainerStatus(pod *corev1.Pod, containerName string) *corev1.ContainerStatus {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == containerName {
			return &cs
		}
	}
	for _, cs := range pod.Status.InitContainerStatuses {
		if cs.Name == containerName {
			return &cs
		}
	}
	return nil
}

func GetContainerState(status *corev1.ContainerStatus) string {
	if status == nil {
		return "Unknown"
	}

	if status.State.Waiting != nil {
		return "Waiting"
	}
	if status.State.Running != nil {
		return "Running"
	}
	if status.State.Terminated != nil {
		return "Terminated"
	}
	return "Unknown"
}

func GetTerminationReason(status *corev1.ContainerStatus) string {
	if status == nil || status.LastTerminationState.Terminated == nil {
		return ""
	}
	return status.LastTerminationState.Terminated.Reason
}

func GetRestartCount(status *corev1.ContainerStatus) int32 {
	if status == nil {
		return 0
	}
	return status.RestartCount
}

func GetLastTerminationTime(status *corev1.ContainerStatus) time.Time {
	if status == nil || status.LastTerminationState.Terminated == nil {
		return time.Time{}
	}
	return status.LastTerminationState.Terminated.FinishedAt.Time
}
