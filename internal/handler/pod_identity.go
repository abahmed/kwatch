package handler

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/event"
)

func podLineageID(pod *corev1.Pod) string {
	if pod == nil || pod.Annotations == nil {
		return ""
	}
	return pod.Annotations[event.PodLineageAnnotation]
}
