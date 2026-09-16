package pod

import (
	"fmt"
	"slices"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/monitor/pod/enrichment"
	"github.com/abahmed/kwatch/internal/observe"
)

// PolicyEvaluator owns Pod/container policy and report construction. Queue
// lookup and recovery remain in Runtime; this type only evaluates findings.
type PolicyEvaluator struct {
	runtime    config.RuntimeConfig
	monitor    *Monitor
	sink       monitor.ObservationSink
	lastState  func(string, string, string) *model.ContainerState
	oomTracker *oomTracker
	now        func() time.Time
}

// NewPolicyEvaluatorWithRuntimeConfig constructs Pod evaluation from the
// immutable runtime snapshot.
func NewPolicyEvaluatorWithRuntimeConfig(
	runtime config.RuntimeConfig,
	sink monitor.ObservationSink,
	lastState func(string, string, string) *model.ContainerState,
	runtimeClock clock.Clock,
) *PolicyEvaluator {
	runtimeClock = clock.Require(runtimeClock)
	var tracker *oomTracker
	if runtime.Monitors().OOM().Enabled {
		tracker = newOOMTracker(
			runtime.Monitors().OOM().Threshold,
			time.Duration(runtime.Monitors().OOM().WindowMinutes)*time.Minute,
			runtimeClock,
		)
	}
	evaluator := &PolicyEvaluator{
		runtime:    runtime,
		monitor:    NewWithRuntimeConfig(runtime),
		sink:       sink,
		lastState:  lastState,
		oomTracker: tracker,
		now:        runtimeClock.Now,
	}
	return evaluator
}

// EvaluatePod evaluates a Pod-level finding after policy detection.
func (e *PolicyEvaluator) EvaluatePod(ctx *enrichment.Context) {
	if ctx == nil || ctx.Pod == nil {
		return
	}
	if e.lastState != nil {
		ctx.PodLastState = e.lastState(ctx.Pod.Namespace, ctx.Pod.Name, ".")
	}
	if !e.monitor.DetectPod(ctx) {
		return
	}
	e.loadPodEvents(ctx)
	if e.monitor.EnrichPod(ctx) || !ctx.PodHasIssues {
		return
	}
	owner := contextOwner(ctx)
	klog.V(2).InfoS(
		"pod only issue",
		"component", "monitor/pod",
		"operation", "evaluate",
		"pod", ctx.Pod.Name,
		"owner", owner.Name,
		"reason", ctx.PodReason,
	)
	hint, facts := e.podIssueHint(ctx)
	obs := observe.PodOwnedBy(
		ctx.Pod, ".", ctx.PodReason, owner,
	).WithHint(hint).WithFacts(facts).WithEvidence(
		"",
		event.FormatPodEvents(ctx.Events),
		&model.ContainerState{
			Reason: ctx.PodReason,
			Msg:    ctx.PodMsg,
		},
	)
	e.observe(obs)
}

// EvaluateContainers evaluates all init and regular container statuses.
func (e *PolicyEvaluator) EvaluateContainers(ctx *enrichment.Context) {
	if ctx == nil || ctx.Pod == nil {
		return
	}
	containers := make([]*corev1.ContainerStatus, 0)
	initContainers := make(map[string]bool)
	for index := range ctx.Pod.Status.InitContainerStatuses {
		container := &ctx.Pod.Status.InitContainerStatuses[index]
		containers = append(containers, container)
		initContainers[container.Name] = true
	}
	for index := range ctx.Pod.Status.ContainerStatuses {
		containers = append(containers, &ctx.Pod.Status.ContainerStatuses[index])
	}
	for _, container := range containers {
		ctx.Container = &enrichment.ContainerContext{
			Container: container,
			LastState: e.containerState(ctx, container.Name),
			IsInit:    initContainers[container.Name],
		}
		hasFinding := e.monitor.DetectContainer(ctx)
		if !hasFinding {
			if e.highRestartEnabled(ctx) {
				e.emitHighRestartAlert(ctx, container)
			}
			continue
		}
		e.loadPodEvents(ctx)
		if e.monitor.SuppressContainer(ctx) {
			hasFinding = false
		}
		e.monitor.EnrichContainerData(ctx)
		if !hasFinding {
			continue
		}
		owner := contextOwner(ctx)
		klog.V(2).InfoS(
			"container only issue",
			"component", "monitor/pod",
			"operation", "evaluate-container",
			"container", container.Name,
			"pod", ctx.Pod.Name,
			"owner", owner.Name,
			"reason", ctx.Container.Reason,
		)
		hint, facts := e.buildContainerHint(ctx)
		obs := observe.PodOwnedBy(
			ctx.Pod, container.Name, ctx.Container.Reason, owner,
		).WithHint(hint).WithFacts(facts).
			WithMessage(ctx.Container.Msg).
			WithEvidence(
				ctx.Container.Logs,
				event.FormatPodEvents(ctx.Events),
				&model.ContainerState{
					RestartCount:     container.RestartCount,
					LastTerminatedOn: ctx.Container.LastTerminatedOn,
					Reason:           ctx.Container.Reason,
					Msg:              ctx.Container.Msg,
					ExitCode:         ctx.Container.ExitCode,
					Status:           ctx.Container.Status,
				},
			)
		obs.Image = container.Image
		obs.RestartCount = container.RestartCount
		e.observe(obs)
	}
}

func (e *PolicyEvaluator) containerState(
	ctx *enrichment.Context, container string,
) *model.ContainerState {
	if e.lastState == nil {
		return nil
	}
	return e.lastState(ctx.Pod.Namespace, ctx.Pod.Name, container)
}

func (e *PolicyEvaluator) observe(obs *model.Observation) {
	if obs == nil || e.sink == nil {
		return
	}
	obs.IncludeEvents = e.runtime.Monitors().IncludeEvents()
	obs.IncludeLogs = e.runtime.Monitors().IncludeLogs()
	e.sink.Process(obs)
}

func contextOwner(ctx *enrichment.Context) model.ObjectRef {
	if ctx.Owner != nil {
		return model.ObjectRef{
			Kind: ctx.Owner.Kind, Namespace: ctx.Pod.Namespace,
			Name: ctx.Owner.Name,
		}
	}
	if len(ctx.Pod.OwnerReferences) == 0 {
		return observe.SelfOwner("Pod", ctx.Pod.Namespace, ctx.Pod.Name)
	}
	return model.ObjectRef{}
}

func (e *PolicyEvaluator) highRestartEnabled(ctx *enrichment.Context) bool {
	threshold := e.runtime.Monitors().ContainerRestartThreshold()
	return ctx.Container != nil && threshold > 0 &&
		int(ctx.Container.Container.RestartCount) >= threshold &&
		!podTerminatingOrDisrupted(ctx.Pod) &&
		!e.highRestartSuppressed(ctx)
}

func podTerminatingOrDisrupted(pod *corev1.Pod) bool {
	if pod == nil || pod.DeletionTimestamp != nil {
		return pod != nil && pod.DeletionTimestamp != nil
	}
	if pod.Status.Phase == corev1.PodFailed &&
		pod.Status.Reason == constant.ReasonEvicted {
		return true
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == "DisruptionTarget" {
			return true
		}
	}
	return false
}

func (e *PolicyEvaluator) highRestartSuppressed(
	ctx *enrichment.Context,
) bool {
	reason := ctx.Container.Reason
	if reason == "" {
		terminated := ctx.Container.Container.LastTerminationState.Terminated
		if terminated != nil {
			reason = terminated.Reason
		}
	}
	allowed := e.runtime.Scope().AllowedReasons()
	if len(allowed) > 0 && !slices.Contains(allowed, reason) {
		return true
	}
	forbidden := e.runtime.Scope().ForbiddenReasons()
	if len(forbidden) > 0 && slices.Contains(forbidden, reason) {
		return true
	}
	e.loadPodEvents(ctx)
	return e.monitor.SuppressContainer(ctx)
}

func (e *PolicyEvaluator) loadPodEvents(ctx *enrichment.Context) {
	if ctx.Events != nil || ctx.Pod == nil {
		return
	}
	if ctx.EventsByPod != nil {
		events, err := ctx.EventsByPod(ctx.Pod.Namespace, ctx.Pod.Name)
		if err == nil {
			ctx.Events = sortedEvents(events)
			return
		}
		klog.V(2).InfoS(
			"event index lookup failed, falling back to lister",
			"component", "monitor/pod", "operation", "load-events",
			"pod", ctx.Pod.Name,
		)
	}
	if ctx.EventLister != nil {
		events, err := ctx.EventLister.Events(ctx.Pod.Namespace).
			List(labels.Everything())
		if err == nil {
			filtered := make([]*corev1.Event, 0, len(events))
			for _, event := range events {
				if event.InvolvedObject.Kind == "Pod" &&
					event.InvolvedObject.Name == ctx.Pod.Name {
					filtered = append(filtered, event)
				}
			}
			ctx.Events = sortedEvents(filtered)
		}
		return
	}
	if ctx.Client == nil {
		return
	}
}

func sortedEvents(events []*corev1.Event) *[]corev1.Event {
	items := make([]corev1.Event, 0, len(events))
	for _, event := range events {
		if event != nil {
			items = append(items, *event)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].LastTimestamp.Before(&items[j].LastTimestamp)
	})
	return &items
}

func (e *PolicyEvaluator) podIssueHint(
	ctx *enrichment.Context,
) (string, model.Facts) {
	var facts model.Facts
	hint := enricher.HintForReason(ctx.PodReason)
	if ctx.PodMsg != "" {
		hint = ctx.PodMsg + " — " + hint
	}
	if ctx.PodReason != "Unschedulable" {
		return hint, facts
	}
	if e.runtime.Monitors().Schedule().Enabled {
		if delay := e.unschedulableDelay(ctx); delay > 30*time.Second {
			facts.SchedulingDelay = delay
			hint = fmt.Sprintf(
				"unschedulable for %s — ", format.Duration(delay),
			) + hint
		}
	}
	for _, container := range ctx.Pod.Spec.Containers {
		if request := containerRequestSummary(container); request != "" {
			facts.ResourceRequests = append(facts.ResourceRequests, request)
			hint += "; " + request
		}
	}
	return hint, facts
}

func (e *PolicyEvaluator) unschedulableDelay(
	ctx *enrichment.Context,
) time.Duration {
	for _, condition := range ctx.Pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled &&
			condition.Status == corev1.ConditionFalse {
			if delay := e.now().Sub(condition.LastTransitionTime.Time); delay > 0 {
				return delay
			}
			break
		}
	}
	return e.now().Sub(ctx.Pod.CreationTimestamp.Time)
}

func containerRequestSummary(container corev1.Container) string {
	requests := container.Resources.Requests
	if requests == nil {
		return ""
	}
	cpu, memory := requests.Cpu(), requests.Memory()
	if cpu == nil && memory == nil {
		return ""
	}
	result := container.Name + " requests:"
	if cpu != nil && !cpu.IsZero() {
		result += " cpu=" + cpu.String()
	}
	if memory != nil && !memory.IsZero() {
		result += " mem=" + memory.String()
	}
	return result
}
