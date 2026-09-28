package cluster

import (
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/observe"
)

const (
	genericEventMaxAge = 15 * time.Minute
	caSustainedMinutes = 5
)

var warningEventReasons = map[string]struct{}{
	"BackoffLimitExceeded": {}, "DeadlineExceeded": {},
	"FailedAttachVolume": {}, "FailedBinding": {}, "FailedMount": {},
	"FailedProvision": {}, "ProvisioningFailed": {},
	"VolumeResizeFailed": {}, "FailedScheduling": {},
	"FailedCreate": {}, "FailedDaemonPod": {}, "FailedScale": {},
	"FailedGetMetrics": {}, "FailedGetResourceMetric": {},
	"FailedComputeMetricsReplicas": {}, "FailedRescale": {},
	"FailedCallingWebhook": {}, "FailedAdmissionWebhook": {},
	"FailedValidation": {}, "FailedDiscoveryCheck": {},
	"FailedUpdateEndpointSlices": {}, "NetworkNotReady": {},
	"NodeNotReady": {}, "KubeletNotReady": {}, "Unhealthy": {},
}

var caScaleUpSuccessReasons = map[string]struct{}{
	"TriggeredScaleUp": {}, "ScaledUpGroup": {},
}

// EventRuntime converts failure-shaped Kubernetes Events into observations.
// It owns only Event policy and its small amount of sustain state.
type EventRuntime struct {
	runtime config.RuntimeConfig
	sink    monitor.ObservationSink

	mu        sync.Mutex
	now       func() time.Time
	caBlocked map[string]time.Time
	// caLastSeen is when each failure reason was last reported.
	caLastSeen map[string]time.Time
}

// ProcessWarningEvent handles recent resource-level Warning Events.
func (r *EventRuntime) ProcessWarningEvent(ev *corev1.Event) {
	if ev == nil || ev.Type != corev1.EventTypeWarning ||
		ev.InvolvedObject.Kind == "Pod" ||
		ev.Source.Component == "cluster-autoscaler" {
		return
	}
	if _, ok := warningEventReasons[ev.Reason]; !ok ||
		!r.recentEvent(ev, r.nowTime()) {
		return
	}
	hint := ev.Reason
	if ev.Message != "" {
		hint += ": " + ev.Message
	}
	if ev.Source.Component != "" {
		hint += " (source: " + ev.Source.Component + ")"
	}
	observation := observe.ObjectNamed(
		strings.ToLower(ev.InvolvedObject.Kind),
		ev.InvolvedObject.Namespace,
		ev.InvolvedObject.Name,
		ev.Reason,
	).WithSeverity(model.SeverityWarning).WithHint(hint).
		WithMessage(ev.Message)
	if facts := eventFacts(ev); !facts.IsZero() {
		observation.WithFacts(facts)
	}
	observation.Transient = true
	r.process(observation)
}

// ProcessClusterAutoscalerEvent handles sustained Cluster Autoscaler failures.
func (r *EventRuntime) ProcessClusterAutoscalerEvent(ev *corev1.Event) {
	if ev == nil || !r.recentEvent(ev, r.nowTime()) {
		return
	}
	if ev.Reason == "FailedToScaleUp" || ev.Reason == "NotTriggerScaleUp" {
		now := r.nowTime()
		first := r.markCA(ev.Reason, now)
		if now.Sub(first) < caSustainedMinutes*time.Minute {
			return
		}
		hint := ev.Message
		if hint == "" {
			hint = "Cluster autoscaler cannot scale: " + ev.Reason
		}
		observation := observe.Synthetic(
			"cluster-autoscaler", "cluster-autoscaler", ev.Reason,
		).WithSeverity(model.SeverityWarning).WithHint(hint).
			WithMessage(ev.Message).
			WithFacts(autoscalerEventFacts(strings.ToLower(ev.Message)))
		// The involved object is usually the pending Pod, not a node.
		if ev.InvolvedObject.Kind == "Node" {
			observation.NodeName = ev.InvolvedObject.Name
		}
		observation.Transient = true
		r.process(observation)
		return
	}
	// Only a successful scale-up clears the failure. Unrelated autoscaler
	// events (scale-down, status updates) used to resolve it every few
	// seconds, so the incident flapped.
	if _, ok := caScaleUpSuccessReasons[ev.Reason]; !ok {
		return
	}
	r.clearCA()
	if r.sink == nil {
		return
	}
	autoscaler := model.ObjectRef{
		Kind: "cluster-autoscaler", Name: "cluster-autoscaler",
	}
	r.sink.Resolve(autoscaler, "FailedToScaleUp")
	r.sink.Resolve(autoscaler, "NotTriggerScaleUp")
}

func (r *EventRuntime) process(observation *model.Observation) {
	if observation == nil || r.sink == nil {
		return
	}
	observation.IncludeEvents = r.runtime.Monitors().IncludeEvents()
	observation.IncludeLogs = r.runtime.Monitors().IncludeLogs()
	r.sink.Process(observation)
}

func (r *EventRuntime) markCA(reason string, now time.Time) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	// A failure not repeated within the sustain window starts a new
	// window, so an old failure does not make a new one alert at once.
	first, ok := r.caBlocked[reason]
	if !ok || now.Sub(r.caLastSeen[reason]) > caSustainedMinutes*time.Minute {
		first = now
		r.caBlocked[reason] = first
	}
	if r.caLastSeen == nil {
		r.caLastSeen = make(map[string]time.Time)
	}
	r.caLastSeen[reason] = now
	return first
}

func (r *EventRuntime) clearCA() {
	r.mu.Lock()
	delete(r.caBlocked, "FailedToScaleUp")
	delete(r.caBlocked, "NotTriggerScaleUp")
	clear(r.caLastSeen)
	r.mu.Unlock()
}

func (r *EventRuntime) nowTime() time.Time {
	r.mu.Lock()
	now := r.now
	r.mu.Unlock()
	return now()
}

func (r *EventRuntime) recentEvent(ev *corev1.Event, now time.Time) bool {
	t := eventTime(ev)
	return !t.IsZero() && !t.After(now) && now.Sub(t) <= genericEventMaxAge
}

func eventTime(ev *corev1.Event) time.Time {
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	if ev.Series != nil && !ev.Series.LastObservedTime.IsZero() {
		return ev.Series.LastObservedTime.Time
	}
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.FirstTimestamp.IsZero() {
		return ev.FirstTimestamp.Time
	}
	return ev.CreationTimestamp.Time
}
