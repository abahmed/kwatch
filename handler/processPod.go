package handler

import (
	"github.com/abahmed/kwatch/filter"
	corev1 "k8s.io/api/core/v1"
)

func (h *handler) ProcessPod(eventType string, pod *corev1.Pod) {
	if pod == nil {
		return
	}

	if eventType == "DELETED" {
		h.memory.DelPod(pod.Namespace, pod.Name)
		return
	}

	ctx := filter.Context{
		Client: h.kclient,
		Config: h.config,
		Memory: h.memory,
		Pod:    pod,
		EvType: eventType,
	}

	h.executePodFilters(&ctx)
	h.executeContainersFilters(&ctx)
}
