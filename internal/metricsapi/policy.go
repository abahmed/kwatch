package metricsapi

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (m *Monitor) processMetric(
	pod *corev1.Pod,
	containerName string,
	usage map[string]string,
	limit corev1.ResourceList,
	seen map[string]bool,
) {
	seen[containerName] = true
	owner := m.podOwner(pod)
	if usageMemory, ok := parseQuantity(
		usage[string(corev1.ResourceMemory)],
	); ok {
		sig := usageSignal(
			pod, containerName, usageMemory, limit.Memory(),
			m.cfg.MemoryWarningPercent, m.cfg.MemoryCriticalPercent,
			constant.ReasonContainerMemoryHigh, "memory", owner,
		)
		if sig != nil {
			m.report(sig)
		} else {
			m.resolveContainer(
				pod, containerName, constant.ReasonContainerMemoryHigh,
			)
		}
	}
	if usageCPU, ok := parseQuantity(
		usage[string(corev1.ResourceCPU)],
	); ok {
		sig := usageSignal(
			pod, containerName, usageCPU, limit.Cpu(),
			m.cfg.CPUWarningPercent, m.cfg.CPUCriticalPercent,
			constant.ReasonContainerCPUHigh, "cpu", owner,
		)
		if sig != nil {
			m.report(sig)
		} else {
			m.resolveContainer(
				pod, containerName, constant.ReasonContainerCPUHigh,
			)
		}
	}
}

func usageSignal(
	pod *corev1.Pod,
	container string,
	usage, limit *resource.Quantity,
	warning, critical int,
	reason, unit string,
	owner model.ObjectRef,
) *model.Observation {
	if limit == nil || limit.IsZero() || usage == nil {
		return nil
	}
	percent := usage.AsApproximateFloat64() /
		limit.AsApproximateFloat64() * 100
	if percent < float64(warning) {
		return nil
	}
	severity := model.SeverityWarning
	if percent >= float64(critical) {
		severity = model.SeverityCritical
	}
	return observe.PodOwnedBy(pod, container, reason, owner).
		WithSeverity(severity).
		WithHint(fmt.Sprintf(
			"container %s %s usage is %.0f%% of its limit (%s/%s)",
			container, unit, percent, usage.String(), limit.String(),
		))
}

func containerLimits(pod *corev1.Pod) map[string]corev1.ResourceList {
	limits := make(map[string]corev1.ResourceList)
	containers := append(
		append([]corev1.Container{}, pod.Spec.InitContainers...),
		pod.Spec.Containers...,
	)
	for _, container := range containers {
		limits[container.Name] = container.Resources.Limits
	}
	return limits
}

func parseQuantity(value string) (*resource.Quantity, bool) {
	if value == "" {
		return nil, false
	}
	quantity, err := resource.ParseQuantity(value)
	return &quantity, err == nil
}

func (m *Monitor) report(obs *model.Observation) {
	m.incidentSink.Process(obs)
}

func (m *Monitor) resolveContainer(pod *corev1.Pod, container, reason string) {
	m.incidentSink.ResolveObserved(
		observe.PodOwnedBy(pod, container, reason, m.podOwner(pod)),
	)
}
