package handler

import (
	"github.com/abahmed/kwatch/filter"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
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

	podEvents, err := util.GetPodEvents(ctx.Client, ctx.Pod.Name, ctx.Pod.Namespace)
	if err != nil {
		logrus.Errorf(
			"failed to get events for pod %s(%s): %s",
			ctx.Pod.Name,
			ctx.Pod.Namespace,
			err.Error())
	}

	if podEvents != nil {
		ctx.Events = &podEvents.Items
	}

	h.executePodFilters(&ctx)
	h.executeContainersFilters(&ctx)
}
