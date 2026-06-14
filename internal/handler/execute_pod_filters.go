package handler

import (
	"sort"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

func (h *handler) executePodFilters(ctx *filter.Context) {
	ctx.PodLastState = h.correlator.GetLastContainerState(
		ctx.Pod.Namespace, ctx.Pod.Name, ".")

	// Phase 1: Detect (pure, no I/O)
	for i := range h.podDetectors {
		if h.podDetectors[i].Detect(ctx) == filter.StatusSkip {
			return
		}
	}

	if !ctx.PodHasIssues || ctx.ContainersHasIssues {
		return
	}

	// Phase 2: Enrich (I/O: events, owner)
	if ctx.Events == nil {
		if ctx.EventLister != nil {
			all, err := ctx.EventLister.Events(ctx.Pod.Namespace).List(labels.Everything())
			if err != nil {
				klog.ErrorS(err, "event lister failed", "pod", ctx.Pod.Name)
			} else {
				items := make([]corev1.Event, 0, len(all))
				for _, e := range all {
					if e.InvolvedObject.Kind == "Pod" && e.InvolvedObject.Name == ctx.Pod.Name {
						items = append(items, *e)
					}
				}
				sort.Slice(items, func(i, j int) bool {
					return items[i].LastTimestamp.Before(&items[j].LastTimestamp)
				})
				ctx.Events = &items
			}
		} else {
			podEvents, err := k8s.GetPodEvents(ctx.Client, ctx.Pod.Name, ctx.Pod.Namespace)
			if err != nil {
				klog.ErrorS(err, "failed to fetch pod events", "pod", ctx.Pod.Name)
			}
			if podEvents != nil {
				ctx.Events = &podEvents.Items
			}
		}
	}

	for i := range h.podEnrichers {
		if h.podEnrichers[i].Enrich(ctx) {
			return
		}
	}

	if !ctx.PodHasIssues {
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

	h.signalEvent(&event.Signal{
		Resource:  "pod",
		PodName:   ctx.Pod.Name,
		Container: ".",
		Namespace: ctx.Pod.Namespace,
		NodeName:  ctx.Pod.Spec.NodeName,
		Reason:    ctx.PodReason,
		Events:    k8s.GetPodEventsStr(ctx.Events),
		Labels:    ctx.Pod.Labels,
		OwnerKind: ownerKind,
		Hint:      enricher.HintForReason(ctx.PodReason),
		Owner:     ownerName,
		ContainerState: &model.ContainerState{
			Reason: ctx.PodReason,
			Msg:    ctx.PodMsg,
			Status: "",
		},
	})
}
