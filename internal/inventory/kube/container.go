package kube

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/abahmed/kwatch/internal/inventory"
)

// ContainerID identifies a container entity: "<pod>/<container>" in the
// pod's namespace.
func ContainerID(namespace, pod, container string) inventory.EntityID {
	return inventory.CoreID(KindContainer, namespace, pod+"/"+container)
}

func containerDescriptions(
	pod *corev1.Pod, podID inventory.EntityID,
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
		status, hasStatus := statuses[c.Name]
		if hasStatus {
			containerStatusAttributes(attrs, status)
		}
		if init && !flagAttr(attrs, AttrSidecar) {
			if !hasStatus {
				setInitAttributes(attrs, c, nil, pod.Namespace)
			} else {
				setInitAttributes(attrs, c, &status, pod.Namespace)
			}
		}
		rel := relations{}
		rel.add(inventory.PartOf, podID)
		rel.add(inventory.Pulls, inventory.CoreID(KindImage, "", c.Image))
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
) map[string]inventory.Value {
	sidecar := init && c.RestartPolicy != nil &&
		*c.RestartPolicy == corev1.ContainerRestartPolicyAlways
	attrs := map[string]inventory.Value{
		AttrImage:   inventory.Text(c.Image),
		AttrInit:    inventory.Bool(init && !sidecar),
		AttrSidecar: inventory.Bool(sidecar),
	}
	setQuantity(attrs, AttrMemoryLimit, c.Resources.Limits.Memory())
	setQuantity(attrs, AttrMemoryReq, c.Resources.Requests.Memory())
	setMilli(attrs, AttrCPULimit, c.Resources.Limits.Cpu())
	setMilli(attrs, AttrCPUReq, c.Resources.Requests.Cpu())
	setQuantity(attrs, AttrEphemeralLimit,
		c.Resources.Limits.StorageEphemeral())
	if budget := probeBudgetSeconds(c); budget > 0 {
		attrs[AttrProbeBudget] = inventory.Number(float64(budget))
	}
	if probes := declaredProbes(c); probes != "" {
		attrs[AttrProbes] = inventory.Text(probes)
	}
	if sc := c.SecurityContext; sc != nil && sc.Privileged != nil &&
		*sc.Privileged {
		attrs[AttrPrivileged] = inventory.Bool(true)
	}
	setPortAttributes(attrs, c)
	setLivenessAttributes(attrs, c)
	setContainerScheduling(attrs, c)
	return attrs
}

// declaredProbes names the probes a container declares.
func declaredProbes(c corev1.Container) string {
	var out []string
	for _, probe := range []struct {
		name  string
		probe *corev1.Probe
	}{{"startup", c.StartupProbe}, {"readiness", c.ReadinessProbe},
		{"liveness", c.LivenessProbe}} {
		if probe.probe != nil {
			out = append(out, probe.name)
		}
	}
	return strings.Join(out, ",")
}

func containerStatusAttributes(
	attrs map[string]inventory.Value, s corev1.ContainerStatus,
) {
	attrs[AttrReady] = inventory.Bool(s.Ready)
	attrs[AttrRestarts] = inventory.Number(float64(s.RestartCount))
	if s.ImageID != "" {
		attrs[AttrImageID] = inventory.Text(s.ImageID)
	}
	switch {
	case s.State.Running != nil:
		attrs[AttrState] = inventory.Text("running")
		attrs[AttrStartedAt] = inventory.Time(s.State.Running.StartedAt.Time)
	case s.State.Waiting != nil:
		attrs[AttrState] = inventory.Text("waiting")
		attrs[AttrStateReason] = inventory.Text(s.State.Waiting.Reason)
		if s.State.Waiting.Message != "" {
			attrs[AttrMessage] = inventory.Text(
				evidenceText(s.State.Waiting.Message),
			)
		}
	case s.State.Terminated != nil:
		t := s.State.Terminated
		attrs[AttrState] = inventory.Text("terminated")
		attrs[AttrStateReason] = inventory.Text(t.Reason)
		attrs[AttrExitCode] = inventory.Number(float64(t.ExitCode))
		setFinished(attrs, t)
		if t.Message != "" {
			attrs[AttrMessage] = inventory.Text(evidenceText(t.Message))
		}
	}
	if last := s.LastTerminationState.Terminated; last != nil {
		attrs[AttrLastReason] = inventory.Text(last.Reason)
		attrs[AttrLastExitCode] = inventory.Number(float64(last.ExitCode))
		attrs[AttrLastFinished] = inventory.Time(last.FinishedAt.Time)
		if !last.StartedAt.IsZero() {
			attrs[AttrLastStarted] = inventory.Time(last.StartedAt.Time)
		}
		if last.Message != "" {
			attrs[AttrLastMessage] = inventory.Text(evidenceText(last.Message))
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
	attrs map[string]inventory.Value, name string, q *resource.Quantity,
) {
	if q != nil && !q.IsZero() {
		attrs[name] = inventory.Number(float64(q.Value()))
	}
}

func setMilli(
	attrs map[string]inventory.Value, name string, q *resource.Quantity,
) {
	if q != nil && !q.IsZero() {
		attrs[name] = inventory.Number(float64(q.MilliValue()))
	}
}

// resourceDiff reports in-place resize requests: container resources are
// the only mutable part of a running pod's spec.
func resourceDiff(before, after *corev1.Pod) []inventory.FieldChange {
	old := make(map[string]corev1.ResourceRequirements)
	for _, c := range before.Spec.Containers {
		old[c.Name] = c.Resources
	}
	var fields []inventory.FieldChange
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
	fields []inventory.FieldChange,
	container, section string,
	name corev1.ResourceName,
	before, after corev1.ResourceList,
) []inventory.FieldChange {
	b, a := before[name], after[name]
	if b.Cmp(a) == 0 {
		return fields
	}
	return append(fields, inventory.FieldChange{
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

func flagAttr(attrs map[string]inventory.Value, name string) bool {
	set, _ := attrs[name].AsBool()
	return set
}
