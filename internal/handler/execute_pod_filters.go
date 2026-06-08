package handler

import (
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/storage"
	"k8s.io/klog/v2"
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

	klog.InfoS("pod only issue", "pod", ctx.Pod.Name, "owner", ownerName, "reason", ctx.PodReason, "message", ctx.PodMsg)

	ownerKind := ""
	if ctx.Owner != nil {
		ownerKind = ctx.Owner.Kind
	}

	ev := event.Event{
		PodName:       ctx.Pod.Name,
		ContainerName: "",
		Namespace:     ctx.Pod.Namespace,
		NodeName:      ctx.Pod.Spec.NodeName,
		Reason:        ctx.PodReason,
		Events:        k8s.GetPodEventsStr(ctx.Events),
		Logs:          "",
		Labels:        ctx.Pod.Labels,
		OwnerKind:     ownerKind,
	}

	inc, action := h.correlator.Process(ev, ownerName)
	if action != model.ActionSkip {
		h.alertManager.NotifyIncident(inc, action)
	}
}
