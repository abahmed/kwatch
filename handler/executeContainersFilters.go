package handler

import (
	"time"

	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/filter"
	"github.com/abahmed/kwatch/storage"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

func (h *handler) executeContainersFilters(ctx *filter.Context) {
	containers := make([]*corev1.ContainerStatus, 0)
	for idx := range ctx.Pod.Status.InitContainerStatuses {
		containers = append(containers, &ctx.Pod.Status.InitContainerStatuses[idx])
	}
	for idx := range ctx.Pod.Status.ContainerStatuses {
		containers = append(containers, &ctx.Pod.Status.ContainerStatuses[idx])
	}

	for _, container := range containers {
		ctx.Container = &filter.ContainerContext{
			Container:        container,
			HasRestarts:      false,
			LastTerminatedOn: time.Time{},
		}

		isContainerOk := false
		for i := range h.containerFilters {
			if shouldStop := h.containerFilters[i].Execute(ctx); shouldStop {
				isContainerOk = true
				break
			}
		}

		ctx.Memory.AddPodContainer(
			ctx.Pod.Namespace,
			ctx.Pod.Name,
			ctx.Container.Container.Name,
			&storage.ContainerState{
				RestartCount:     ctx.Container.Container.RestartCount,
				LastTerminatedOn: ctx.Container.LastTerminatedOn,
				Reason:           ctx.Container.Reason,
				Msg:              ctx.Container.Msg,
				ExitCode:         ctx.Container.ExitCode,
				Status:           ctx.Container.Status,
			})

		if !isContainerOk {
			ownerName := ""
			if ctx.Owner != nil {
				ownerName = ctx.Owner.Name
			}

			logrus.Printf(
				"container only issue %s %s %s %s %s %d",
				ctx.Container.Container.Name,
				ctx.Pod.Name,
				ownerName,
				ctx.Container.Reason,
				ctx.Container.Msg,
				ctx.Container.ExitCode)

			h.alertManager.NotifyEvent(event.Event{
				PodName:       ctx.Pod.Name,
				ContainerName: ctx.Container.Container.Name,
				Namespace:     ctx.Pod.Namespace,
				Reason:        ctx.Container.Reason,
				Events:        util.GetPodEventsStr(ctx.Events),
				Logs:          ctx.Container.Logs,
				Labels:        ctx.Pod.Labels,
			})
		}
	}
}
