package handler

import (
	"time"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
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
			LastState: h.correlator.GetLastContainerState(
				ctx.Pod.Namespace, ctx.Pod.Name, container.Name),
		}

		// Phase 1: Detect (pure, no I/O)
		broken := false
		for i := range h.containerDetectors {
			if h.containerDetectors[i].Detect(ctx) == filter.StatusSkip {
				broken = false
				break
			}
			broken = true
		}

		if !broken {
			continue
		}

		// Phase 2: Enrich (I/O: events, owner, logs)
		if ctx.Events == nil {
			podEvents, err := k8s.GetPodEvents(ctx.Client, ctx.Pod.Name, ctx.Pod.Namespace)
			if err != nil {
				klog.ErrorS(err, "failed to fetch pod events", "pod", ctx.Pod.Name)
			}
			if podEvents != nil {
				ctx.Events = &podEvents.Items
			}
		}

		for i := range h.containerEnrichers {
			if h.containerEnrichers[i].Enrich(ctx) {
				broken = false
				break
			}
		}

		if !broken {
			continue
		}

		ownerName := ""
		if ctx.Owner != nil {
			ownerName = ctx.Owner.Name
		}

		klog.InfoS(
			"container only issue",
			"container", ctx.Container.Container.Name,
			"pod", ctx.Pod.Name,
			"owner", ownerName,
			"reason", ctx.Container.Reason,
			"message", ctx.Container.Msg,
			"exitCode", ctx.Container.ExitCode)

		ownerKind := ""
		if ctx.Owner != nil {
			ownerKind = ctx.Owner.Kind
		}

		ev := event.Event{
			PodName:       ctx.Pod.Name,
			ContainerName: ctx.Container.Container.Name,
			Namespace:     ctx.Pod.Namespace,
			NodeName:      ctx.Pod.Spec.NodeName,
			Reason:        ctx.Container.Reason,
			Events:        k8s.GetPodEventsStr(ctx.Events),
			Logs:          ctx.Container.Logs,
			Labels:        ctx.Pod.Labels,
			OwnerKind:     ownerKind,
			RestartCount:  int(ctx.Container.Container.RestartCount),
		}

		cs := &model.ContainerState{
			RestartCount:     ctx.Container.Container.RestartCount,
			LastTerminatedOn: ctx.Container.LastTerminatedOn,
			Reason:           ctx.Container.Reason,
			Msg:              ctx.Container.Msg,
			ExitCode:         ctx.Container.ExitCode,
			Status:           ctx.Container.Status,
		}
		inc, action := h.correlator.Process(ev, ownerName, cs)
		if action != model.ActionSkip {
			h.alertManager.NotifyIncident(inc, action)
		}
	}
}
