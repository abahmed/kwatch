package handler

import (
	"time"

	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/filter"
	"github.com/abahmed/kwatch/storage"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
)

func (h *handler) executeContainersFilters(ctx *filter.Context) {
	for cIdx := range ctx.Pod.Status.ContainerStatuses {
		ctx.Container = &filter.ContainerContext{
			Container:        &ctx.Pod.Status.ContainerStatuses[cIdx],
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

			if ctx.Events == nil {
				events, _ := util.GetPodEvents(ctx.Client, ctx.Pod.Name, ctx.Pod.Namespace)
				ctx.Events = &events.Items
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
