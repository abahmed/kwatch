package kube

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// ContainerID identifies a container entity: "<pod>/<container>" in the
// pod's namespace.
func ContainerID(namespace, pod, container string) knowledge.EntityID {
	return knowledge.NewEntityID(KindContainer, namespace, pod+"/"+container)
}

func containerDescriptions(
	pod *corev1.Pod, podID knowledge.EntityID,
) []Description {
	statuses := make(map[string]corev1.ContainerStatus)
	for _, s := range pod.Status.InitContainerStatuses {
		statuses[s.Name] = s
	}
	for _, s := range pod.Status.ContainerStatuses {
		statuses[s.Name] = s
	}
	var out []Description
	forEachContainer(pod, func(c corev1.Container, init bool) {
		attrs := containerSpecAttributes(c, init)
		if status, ok := statuses[c.Name]; ok {
			containerStatusAttributes(attrs, status)
		}
		rel := relations{}
		rel.add(knowledge.PartOf, podID)
		rel.add(knowledge.Pulls, knowledge.NewEntityID(KindImage, "", c.Image))
		out = append(out, Description{
			ID:         ContainerID(pod.Namespace, pod.Name, c.Name),
			Attributes: attrs,
			Relations:  rel,
		})
	})
	return out
}

func containerSpecAttributes(
	c corev1.Container, init bool,
) map[string]knowledge.Value {
	sidecar := init && c.RestartPolicy != nil &&
		*c.RestartPolicy == corev1.ContainerRestartPolicyAlways
	attrs := map[string]knowledge.Value{
		AttrImage:   knowledge.Text(c.Image),
		AttrInit:    knowledge.Bool(init && !sidecar),
		AttrSidecar: knowledge.Bool(sidecar),
	}
	setQuantity(attrs, AttrMemoryLimit, c.Resources.Limits.Memory())
	setQuantity(attrs, AttrMemoryReq, c.Resources.Requests.Memory())
	setMilli(attrs, AttrCPULimit, c.Resources.Limits.Cpu())
	setMilli(attrs, AttrCPUReq, c.Resources.Requests.Cpu())
	setQuantity(attrs, AttrEphemeralLimit,
		c.Resources.Limits.StorageEphemeral())
	if budget := probeBudgetSeconds(c); budget > 0 {
		attrs[AttrProbeBudget] = knowledge.Number(float64(budget))
	}
	return attrs
}

func containerStatusAttributes(
	attrs map[string]knowledge.Value, s corev1.ContainerStatus,
) {
	attrs[AttrReady] = knowledge.Bool(s.Ready)
	attrs[AttrRestarts] = knowledge.Number(float64(s.RestartCount))
	switch {
	case s.State.Running != nil:
		attrs[AttrState] = knowledge.Text("running")
		attrs[AttrStartedAt] = knowledge.Time(s.State.Running.StartedAt.Time)
	case s.State.Waiting != nil:
		attrs[AttrState] = knowledge.Text("waiting")
		attrs[AttrStateReason] = knowledge.Text(s.State.Waiting.Reason)
		if s.State.Waiting.Message != "" {
			attrs[AttrMessage] = knowledge.Text(
				truncate(s.State.Waiting.Message),
			)
		}
	case s.State.Terminated != nil:
		t := s.State.Terminated
		attrs[AttrState] = knowledge.Text("terminated")
		attrs[AttrStateReason] = knowledge.Text(t.Reason)
		attrs[AttrExitCode] = knowledge.Number(float64(t.ExitCode))
		if t.Message != "" {
			attrs[AttrMessage] = knowledge.Text(truncate(t.Message))
		}
	}
	if last := s.LastTerminationState.Terminated; last != nil {
		attrs[AttrLastReason] = knowledge.Text(last.Reason)
		attrs[AttrLastExitCode] = knowledge.Number(float64(last.ExitCode))
		attrs[AttrLastFinished] = knowledge.Time(last.FinishedAt.Time)
		if last.Message != "" {
			attrs[AttrLastMessage] = knowledge.Text(truncate(last.Message))
		}
	}
}

// probeBudgetSeconds is how long the container's startup or readiness
// probe allows before failing: initial delay plus every permitted retry.
func probeBudgetSeconds(c corev1.Container) int32 {
	probe := c.StartupProbe
	if probe == nil {
		probe = c.ReadinessProbe
	}
	if probe == nil {
		return 0
	}
	period, failures := probe.PeriodSeconds, probe.FailureThreshold
	if period <= 0 {
		period = 10
	}
	if failures <= 0 {
		failures = 3
	}
	return probe.InitialDelaySeconds + period*failures
}

func setQuantity(
	attrs map[string]knowledge.Value, name string, q *resource.Quantity,
) {
	if q != nil && !q.IsZero() {
		attrs[name] = knowledge.Number(float64(q.Value()))
	}
}

func setMilli(
	attrs map[string]knowledge.Value, name string, q *resource.Quantity,
) {
	if q != nil && !q.IsZero() {
		attrs[name] = knowledge.Number(float64(q.MilliValue()))
	}
}

// resourceDiff reports in-place resize requests: container resources are
// the only mutable part of a running pod's spec.
func resourceDiff(before, after *corev1.Pod) []knowledge.FieldChange {
	old := make(map[string]corev1.ResourceRequirements)
	for _, c := range before.Spec.Containers {
		old[c.Name] = c.Resources
	}
	var fields []knowledge.FieldChange
	for _, c := range after.Spec.Containers {
		previous, ok := old[c.Name]
		if !ok {
			continue
		}
		for _, name := range []corev1.ResourceName{
			corev1.ResourceCPU, corev1.ResourceMemory,
		} {
			fields = appendQuantityChange(fields, c.Name, "limits", name,
				previous.Limits, c.Resources.Limits)
			fields = appendQuantityChange(fields, c.Name, "requests", name,
				previous.Requests, c.Resources.Requests)
		}
	}
	return fields
}

func appendQuantityChange(
	fields []knowledge.FieldChange,
	container, section string,
	name corev1.ResourceName,
	before, after corev1.ResourceList,
) []knowledge.FieldChange {
	b, a := before[name], after[name]
	if b.Cmp(a) == 0 {
		return fields
	}
	return append(fields, knowledge.FieldChange{
		Path: "containers[" + container + "].resources." + section +
			"." + string(name),
		Before: quantityText(b),
		After:  quantityText(a),
	})
}

func quantityText(q resource.Quantity) string {
	if q.IsZero() {
		return ""
	}
	return q.String()
}
