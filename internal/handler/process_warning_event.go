package handler

import (
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

const genericEventMaxAge = 15 * time.Minute

// warningEventReasons contains failure reasons emitted for resources other
// than Pods. The allowlist is intentional: Kubernetes also emits Warning
// events for recoverable or informational transitions, and treating every
// Warning as an incident creates more noise than coverage.
var warningEventReasons = map[string]struct{}{
	"BackoffLimitExceeded": {}, "DeadlineExceeded": {},
	"FailedAttachVolume": {}, "FailedBinding": {}, "FailedMount": {},
	"FailedProvision": {}, "ProvisioningFailed": {}, "VolumeResizeFailed": {},
	"FailedScheduling": {}, "FailedCreate": {}, "FailedDaemonPod": {},
	"FailedScale": {}, "FailedGetMetrics": {}, "FailedGetResourceMetric": {},
	"FailedComputeMetricsReplicas": {}, "FailedRescale": {},
	"FailedCallingWebhook": {}, "FailedAdmissionWebhook": {},
	"FailedValidation": {}, "FailedDiscoveryCheck": {},
	"FailedUpdateEndpointSlices": {}, "NetworkNotReady": {},
	"NodeNotReady": {}, "KubeletNotReady": {}, "Unhealthy": {},
}

// ProcessWarningEvent turns a recent, known failure-shaped Event into a
// resource-level incident. Pod and Cluster Autoscaler Events have dedicated
// processors with richer context and are deliberately excluded here.
func (h *handler) ProcessWarningEvent(ev *corev1.Event) {
	if ev == nil || ev.Type != corev1.EventTypeWarning ||
		ev.InvolvedObject.Kind == "Pod" || ev.Source.Component == "cluster-autoscaler" {
		return
	}
	if _, ok := warningEventReasons[ev.Reason]; !ok || !recentEvent(ev, h.now()) {
		return
	}
	owner := ev.InvolvedObject.Namespace + "/" + ev.InvolvedObject.Name
	if ev.InvolvedObject.Namespace == "" {
		owner = ev.InvolvedObject.Name
	}
	hint := ev.Reason
	if ev.Message != "" {
		hint += ": " + ev.Message
	}
	if ev.Source.Component != "" {
		hint += " (source: " + ev.Source.Component + ")"
	}
	h.signalEvent(&event.Signal{
		Resource:  strings.ToLower(ev.InvolvedObject.Kind),
		Namespace: ev.InvolvedObject.Namespace,
		PodName:   ev.InvolvedObject.Name,
		Owner:     owner,
		Reason:    ev.Reason,
		Hint:      hint,
		Severity:  model.SeverityWarning,
	})
}

func recentEvent(ev *corev1.Event, now time.Time) bool {
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
