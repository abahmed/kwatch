package handler

import (
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/klog/v2"
)

func (h *handler) executePodFilters(ctx *filter.Context) {
	ctx.PodLastState = h.correlator.GetLastContainerState(
		ctx.Pod.Namespace, ctx.Pod.Name, ".")

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

	klog.InfoS("pod only issue", "pod", ctx.Pod.Name, "owner", ownerName, "reason", ctx.PodReason, "message", ctx.PodMsg)

	ownerKind := ""
	if ctx.Owner != nil {
		ownerKind = ctx.Owner.Kind
	}

	ev := event.Event{
		PodName:       ctx.Pod.Name,
		ContainerName: ".",
		Namespace:     ctx.Pod.Namespace,
		NodeName:      ctx.Pod.Spec.NodeName,
		Reason:        ctx.PodReason,
		Events:        k8s.GetPodEventsStr(ctx.Events),
		Logs:          "",
		Labels:        ctx.Pod.Labels,
		OwnerKind:     ownerKind,
	}

	cs := &model.ContainerState{
		Reason: ctx.PodReason,
		Msg:    ctx.PodMsg,
		Status: "",
	}
	inc, action := h.correlator.Process(ev, ownerName, cs)
	if action != model.ActionSkip {
		h.alertManager.NotifyIncident(inc, action)
	}
}
