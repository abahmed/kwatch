package handler

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
)

func (h *handler) executePodFilters(ctx *filter.Context) {
	ctx.PodLastState = h.correlator.GetLastContainerState(
		ctx.Pod.Namespace, ctx.Pod.Name, ".")

	// Phase 1: Detect (pure, no I/O)
	for i := range h.podDetectors {
		switch h.podDetectors[i].Detect(ctx) {
		case filter.StatusSkip:
			return
		case filter.StatusContinue:
			continue
		}
	}

	if !ctx.PodHasIssues || ctx.ContainersHasIssues {
		return
	}

	// Phase 2: Enrich (I/O: events, owner)
	h.loadPodEvents(ctx)

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
	} else if len(ctx.Pod.OwnerReferences) == 0 {
		// An ownerless Pod is its own logical owner. This keeps independent
		// ownerless Pods separate while allowing generated replacements to be
		// folded by correlation only when the Pod has authoritative identity
		// metadata (UID or explicit lineage).
		ownerName = ctx.Pod.Name
	}

	klog.V(
		2,
	).InfoS(
		"pod only issue",
		"pod",
		ctx.Pod.Name,
		"owner",
		ownerName,
		"reason",
		ctx.PodReason,
		"message",
		ctx.PodMsg,
	)

	ownerKind := ""
	if ctx.Owner != nil {
		ownerKind = ctx.Owner.Kind
	}

	hint, facts := h.podIssueHint(ctx)
	h.signalEvent(&event.Signal{
		Resource:        "pod",
		PodName:         ctx.Pod.Name,
		PodUID:          string(ctx.Pod.UID),
		PodLineageID:    podLineageID(ctx.Pod),
		PodGenerateName: ctx.Pod.GenerateName,
		Container:       ".",
		Namespace:       ctx.Pod.Namespace,
		NodeName:        ctx.Pod.Spec.NodeName,
		Reason:          ctx.PodReason,
		Events:          k8s.GetPodEventsStr(ctx.Events),
		Labels:          ctx.Pod.Labels,
		OwnerKind:       ownerKind,
		Hint:            hint,
		Facts:           facts,
		Owner:           ownerName,
		ContainerState: &model.ContainerState{
			Reason: ctx.PodReason,
			Msg:    ctx.PodMsg,
			Status: "",
		},
	})
}

// podIssueHint builds the hint for pod-level (non-container) issues, adding
// scheduling delay and resource requests for unschedulable pods. The same
// details come back as structured facts for the renderer.
func (h *handler) podIssueHint(ctx *filter.Context) (string, model.Facts) {
	var facts model.Facts
	hint := enricher.HintForReason(ctx.PodReason)
	if ctx.PodMsg != "" {
		hint = ctx.PodMsg + " — " + hint
	}
	if ctx.PodReason != "Unschedulable" || ctx.Pod == nil {
		return hint, facts
	}

	if h.config.ScheduleMonitor.Enabled {
		if delay := h.unschedulableDelay(ctx); delay > 30*time.Second {
			facts.SchedulingDelay = delay
			hint = fmt.Sprintf(
				"unschedulable for %s — ",
				roundDuration(delay),
			) + hint
		}
	}

	// Add pod resource requests so the user can compare against node capacity
	for _, c := range ctx.Pod.Spec.Containers {
		if r := containerRequestSummary(c); r != "" {
			facts.ResourceRequests = append(facts.ResourceRequests, r)
			hint = hint + "; " + r
		}
	}
	return hint, facts
}

func (h *handler) unschedulableDelay(ctx *filter.Context) time.Duration {
	for _, c := range ctx.Pod.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
			if delay := h.now().Sub(c.LastTransitionTime.Time); delay > 0 {
				return delay
			}
			break
		}
	}
	return h.now().Sub(ctx.Pod.CreationTimestamp.Time)
}

func containerRequestSummary(c corev1.Container) string {
	req := c.Resources.Requests
	if req == nil {
		return ""
	}
	cpu, mem := req.Cpu(), req.Memory()
	if cpu == nil && mem == nil {
		return ""
	}
	r := c.Name + " requests:"
	if cpu != nil && !cpu.IsZero() {
		r += " cpu=" + cpu.String()
	}
	if mem != nil && !mem.IsZero() {
		r += " mem=" + mem.String()
	}
	return r
}

// roundDuration formats a duration for human display: "5m30s", "2h15m", etc.
func roundDuration(d time.Duration) string {
	return format.Duration(d)
}
