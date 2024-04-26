package handler

import (
	"time"

	"github.com/abahmed/kwatch/filter"
	"github.com/abahmed/kwatch/storage"
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

			logrus.Printf(
				"container only issue %s %s %s %s %s %d",
				ctx.Container.Container.Name,
				ctx.Pod.Name,
				ownerName,
				ctx.Container.Reason,
				ctx.Container.Msg,
				ctx.Container.ExitCode)
		}
	}
}
