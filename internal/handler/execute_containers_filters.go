package handler

import (
	"fmt"
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
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
			if th := h.config.ContainerRestartThreshold; th > 0 &&
				int(container.RestartCount) >= th {
				h.emitHighRestartAlert(ctx, container)
			}
			continue
		}

		// Phase 2: Enrich (I/O: events, owner, logs)
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

		hint := buildContainerHint(ctx)
		ev := h.eventWithConfig(event.Event{
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
			Hint:          hint,
		})

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

// findContainerSpec returns the matching container spec (including init containers) by name.
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

// buildContainerHint computes a rich diagnostic hint from container state + spec.
func buildContainerHint(ctx *filter.Context) string {
	reason := ctx.Container.Reason
	exitCode := ctx.Container.ExitCode

	hint := enricher.HintForReason(reason)

	if exitCode != 0 {
		ecHint := enricher.HintForExitCode(exitCode)
		hint = enricher.CombineHints(hint, ecHint)
	}

	spec := findContainerSpec(ctx.Pod, ctx.Container.Container.Name)

	if reason == "OOMKilled" || exitCode == 137 {
		if spec != nil && spec.Resources.Limits != nil {
			mem := spec.Resources.Limits.Memory()
			if mem != nil && !mem.IsZero() {
				hint = fmt.Sprintf("OOMKilled (memory limit: %s) — consider increasing memory limits", mem.String())
			}
		}
	}

	if spec != nil {
		if reason == "LivenessProbeFailed" || reason == "ReadinessProbeFailed" || reason == "StartupProbeFailed" {
			hint = buildProbeHint(reason, spec)
		} else if reason == "CrashLoopBackOff" && spec.LivenessProbe != nil {
			hint = hint + "; check liveness probe configuration"
		}
	}

	return hint
}

func buildProbeHint(reason string, spec *corev1.Container) string {
	var probe *corev1.Probe
	switch reason {
	case "LivenessProbeFailed":
		probe = spec.LivenessProbe
	case "ReadinessProbeFailed":
		probe = spec.ReadinessProbe
	case "StartupProbeFailed":
		probe = spec.StartupProbe
	}
	if probe == nil {
		return enricher.HintForReason(reason)
	}

	detail := reason
	if probe.HTTPGet != nil {
		detail = fmt.Sprintf("%s (HTTP GET http://%s%s:%d%s)", reason, spec.Name, probe.HTTPGet.Host, probe.HTTPGet.Port.IntValue(), probe.HTTPGet.Path)
	} else if probe.TCPSocket != nil {
		detail = fmt.Sprintf("%s (TCP check :%d)", reason, probe.TCPSocket.Port.IntValue())
	} else if probe.Exec != nil {
		cmd := ""
		if len(probe.Exec.Command) > 0 {
			cmd = probe.Exec.Command[0]
		}
		detail = fmt.Sprintf("%s (exec %s)", reason, cmd)
	}
	return fmt.Sprintf("%s — application not responding to %s probe", detail, probeType(reason))
}

func probeType(reason string) string {
	switch reason {
	case "LivenessProbeFailed":
		return "liveness"
	case "ReadinessProbeFailed":
		return "readiness"
	case "StartupProbeFailed":
		return "startup"
	}
	return "probe"
}

func lastTermInfo(container *corev1.ContainerStatus) (reason string, exitCode int32) {
	if last := container.LastTerminationState.Terminated; last != nil {
		return last.Reason, last.ExitCode
	}
	return "", 0
}

func (h *handler) emitHighRestartAlert(ctx *filter.Context, container *corev1.ContainerStatus) {
	owner := correlation.ResolveOwnerName(ctx.Pod, h.rsLister, h.dsLister, h.ssLister)
	if owner == "" {
		return
	}

	lastReason, lastEC := lastTermInfo(container)

	ev := h.eventWithConfig(event.Event{
		Resource:      "pod",
		PodName:       ctx.Pod.Name,
		ContainerName: container.Name,
		Namespace:     ctx.Pod.Namespace,
		NodeName:      ctx.Pod.Spec.NodeName,
		Reason:        "HighRestartCount",
		Labels:        ctx.Pod.Labels,
		RestartCount:  int(container.RestartCount),
		Hint: fmt.Sprintf("container restarted %d times (last exit: %s, code %d)",
			container.RestartCount, lastReason, lastEC),
	})

	cs := &model.ContainerState{
		RestartCount: container.RestartCount,
		Reason:       lastReason,
		ExitCode:     lastEC,
	}

	inc, action := h.correlator.Process(ev, owner, cs)
	if action != model.ActionSkip {
		h.alertManager.NotifyIncident(inc, action)
	}
}
