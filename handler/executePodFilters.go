package handler

import (
	"github.com/abahmed/kwatch/event"
	"github.com/abahmed/kwatch/filter"
	"github.com/abahmed/kwatch/storage"
	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
)

func (h *handler) executePodFilters(ctx *filter.Context) {
	isPodOk := false
	for i := range h.podFilters {
		if shouldStop := h.podFilters[i].Execute(ctx); shouldStop {
			isPodOk = true
			break
		}
	}

	if isPodOk ||
		ctx.ContainersHasIssues ||
		!ctx.PodHasIssues {
		return
	}

	ownerName := ""
	if ctx.Owner != nil {
		ownerName = ctx.Owner.Name
	}

	ctx.Memory.AddPodContainer(
		ctx.Pod.Namespace,
		ctx.Pod.Name,
		".",
		&storage.ContainerState{
			Reason: ctx.PodReason,
			Msg:    ctx.PodMsg,
			Status: "",
		},
	)

	logrus.Printf("pod only issue %s %s %s %s", ctx.Pod.Name, ownerName, ctx.PodReason, ctx.PodMsg)

	h.alertManager.NotifyEvent(event.Event{
		PodName:       ctx.Pod.Name,
		ContainerName: "",
		Namespace:     ctx.Pod.Namespace,
		Reason:        ctx.PodReason,
		Events:        util.GetPodEventsStr(ctx.Events),
		Logs:          "",
		Labels:        ctx.Pod.Labels,
	})
}
