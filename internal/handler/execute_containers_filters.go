package handler

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/constant"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
)

func (h *handler) executeContainersFilters(ctx *filter.Context) {
	containers := make([]*corev1.ContainerStatus, 0)
	containerIsInit := make(map[string]bool)
	for idx := range ctx.Pod.Status.InitContainerStatuses {
		c := &ctx.Pod.Status.InitContainerStatuses[idx]
		containers = append(containers, c)
		containerIsInit[c.Name] = true
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
			IsInit: containerIsInit[container.Name],
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
			if th := h.config.ContainerRestartThreshold; th > 0 &&
				int(container.RestartCount) >= th &&
				!isPodTerminatingOrDisrupted(ctx.Pod) &&
				!h.highRestartSuppressed(ctx) {
				h.emitHighRestartAlert(ctx, container)
			}
			continue
		}

		// Phase 2: Enrich (I/O: events, owner, logs)
		h.loadPodEvents(ctx)

		for i := range h.containerSuppressionEnrichers {
			if h.containerSuppressionEnrichers[i].Enrich(ctx) {
				broken = false
				break
			}
		}

		// Data enrichers always run — logs and owner info are needed
		// even when the suppression enricher suppresses alerting, so
		// the next notification (after cooldown expires) has fresh data.
		for i := range h.containerDataEnrichers {
			h.containerDataEnrichers[i].Enrich(ctx)
		}

		if !broken {
			continue
		}

		ownerName := ""
		if ctx.Owner != nil {
			ownerName = ctx.Owner.Name
		}

		klog.V(2).InfoS(
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

		hint, facts := h.buildContainerHint(ctx)
		h.signalEvent(&event.Signal{
			Resource:     "pod",
			PodName:      ctx.Pod.Name,
			Container:    ctx.Container.Container.Name,
			Image:        ctx.Container.Container.Image,
			Message:      ctx.Container.Msg,
			Namespace:    ctx.Pod.Namespace,
			NodeName:     ctx.Pod.Spec.NodeName,
			Reason:       ctx.Container.Reason,
			Events:       k8s.GetPodEventsStr(ctx.Events),
			Logs:         ctx.Container.Logs,
			Labels:       ctx.Pod.Labels,
			OwnerKind:    ownerKind,
			RestartCount: ctx.Container.Container.RestartCount,
			Hint:         hint,
			Facts:        facts,
			Owner:        ownerName,
			ContainerState: &model.ContainerState{
				RestartCount:     ctx.Container.Container.RestartCount,
				LastTerminatedOn: ctx.Container.LastTerminatedOn,
				Reason:           ctx.Container.Reason,
				Msg:              ctx.Container.Msg,
				ExitCode:         ctx.Container.ExitCode,
				Status:           ctx.Container.Status,
			},
		})
	}
}

// loadPodEvents populates ctx.Events with the pod's events (oldest first)
// when they are not already loaded. No-op when ctx.Events is already set or
// when the event source is unavailable.
func (h *handler) loadPodEvents(ctx *filter.Context) {
	if ctx.Events != nil || ctx.Pod == nil {
		return
	}
	// Indexed lookup first: O(events for this pod) instead of O(events in the
	// namespace). The informer already maintains this index; until now nothing
	// read it.
	if ctx.EventsByPod != nil {
		evs, err := ctx.EventsByPod(ctx.Pod.Namespace, ctx.Pod.Name)
		if err == nil {
			items := make([]corev1.Event, 0, len(evs))
			for _, e := range evs {
				items = append(items, *e)
			}
			sort.Slice(items, func(i, j int) bool {
				return items[i].LastTimestamp.Before(&items[j].LastTimestamp)
			})
			ctx.Events = &items
			return
		}
		klog.V(
			2,
		).InfoS(
			"event index lookup failed, falling back to lister",
			"pod",
			ctx.Pod.Name,
			"error",
			err,
		)
	}
	if ctx.EventLister != nil {
		all, err := ctx.EventLister.Events(
			ctx.Pod.Namespace,
		).List(
			labels.Everything(),
		)
		if err != nil {
			klog.ErrorS(err, "event lister failed", "pod", ctx.Pod.Name)
			return
		}
		items := make([]corev1.Event, 0, len(all))
		for _, e := range all {
			if e.InvolvedObject.Kind == "Pod" &&
				e.InvolvedObject.Name == ctx.Pod.Name {
				items = append(items, *e)
			}
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].LastTimestamp.Before(&items[j].LastTimestamp)
		})
		ctx.Events = &items
		return
	}
	podEvents, err := k8s.GetPodEvents(
		ctx.Ctx,
		ctx.Client,
		ctx.Pod.Name,
		ctx.Pod.Namespace,
	)
	if err != nil {
		klog.ErrorS(err, "failed to fetch pod events", "pod", ctx.Pod.Name)
		return
	}
	if podEvents != nil {
		ctx.Events = &podEvents.Items
	}
}

// highRestartSuppressed reports whether the HighRestartCount alert must be
// suppressed for this container. The high-restart path only runs when the
// container detectors all returned skip — which includes the reason
// allow/forbid filter — so without these checks it would bypass the user's
// reason allow/forbid configuration and the suppression enrichers (silences,
// log patterns, graceful-shutdown killing) that apply to normal incidents.
func (h *handler) highRestartSuppressed(ctx *filter.Context) bool {
	if ctx.Container == nil {
		return true
	}

	// Use the last termination reason for the allow/forbid check — it is the
	// failure that produced the restart history the alert reports. The
	// detectors leave it empty for a currently-Running container.
	reason := ctx.Container.Reason
	if reason == "" {
		term := ctx.Container.Container.LastTerminationState
		if last := term.Terminated; last != nil {
			reason = last.Reason
		}
	}

	if len(h.config.AllowedReasons) > 0 &&
		!slices.Contains(h.config.AllowedReasons, reason) {
		klog.InfoS(
			"skipping high-restart-count alert as reason is not in the reason "+
				"allow list",
			"reason",
			reason,
		)
		return true
	}
	if len(h.config.ForbiddenReasons) > 0 &&
		slices.Contains(h.config.ForbiddenReasons, reason) {
		klog.InfoS(
			"skipping high-restart-count alert as reason is in the reason "+
				"forbid list",
			"reason",
			reason,
		)
		return true
	}

	h.loadPodEvents(ctx)
	for i := range h.containerSuppressionEnrichers {
		if h.containerSuppressionEnrichers[i].Enrich(ctx) {
			return true
		}
	}
	return false
}

// findContainerSpec returns the matching container spec (including init
// containers) by name.
func findContainerSpec(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == name {
			return &pod.Spec.InitContainers[i]
		}
	}
	return nil
}

// oomKey returns a workload-scoped tracker key so OOM frequency is counted
// across pod restarts. ReplicaSet/DaemonSet pods get a new name on every
// crash, so a pod-scoped key would never reach the repeating threshold for
// crash-looping workloads. Falls back to the pod name for bare pods.
func (h *handler) oomKey(ctx *filter.Context) string {
	if ctx.Owner != nil && ctx.Owner.Name != "" {
		return ctx.Pod.Namespace + "/" + ctx.Owner.Name + "/" +
			ctx.Container.Container.Name
	}
	return ctx.Pod.Namespace + "/" + ctx.Pod.Name + "/" +
		ctx.Container.Container.Name
}

// isPodTerminatingOrDisrupted returns true when the pod is in a terminal or
// terminating state where restart-count alerts should be suppressed
// (eviction, deletion, disruption target). Matches the same conditions as
// the DisruptionFilter to avoid firing HighRestartCount for intentionally
// terminated pods.
func isPodTerminatingOrDisrupted(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.DeletionTimestamp != nil {
		return true
	}
	if pod.Status.Phase == corev1.PodFailed &&
		pod.Status.Reason == constant.ReasonEvicted {
		return true
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == "DisruptionTarget" {
			return true
		}
	}
	return false
}

func (h *handler) emitHighRestartAlert(
	ctx *filter.Context,
	container *corev1.ContainerStatus,
) {
	owner := correlation.ResolveOwnerName(
		ctx.Pod,
		h.listers.RS,
		h.listers.DS,
		h.listers.SS,
	)
	if owner == "" {
		return
	}

	lastReason, lastEC := lastTermInfo(container)

	h.signalEvent(&event.Signal{
		Resource:     "pod",
		PodName:      ctx.Pod.Name,
		Container:    container.Name,
		Image:        container.Image,
		Namespace:    ctx.Pod.Namespace,
		NodeName:     ctx.Pod.Spec.NodeName,
		Reason:       constant.ReasonHighRestartCount,
		Labels:       ctx.Pod.Labels,
		RestartCount: container.RestartCount,
		Hint: fmt.Sprintf(
			"container restarted %d times (last exit: %s, code %d)",
			container.RestartCount,
			lastReason,
			lastEC,
		),
		Owner: owner,
		ContainerState: &model.ContainerState{
			RestartCount: container.RestartCount,
			Reason:       lastReason,
			ExitCode:     lastEC,
		},
	})
}
