package kubeletmetrics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/model"
)

func (m *Monitor) checkSummary(
	ctx context.Context, node *corev1.Node, pods map[string]*corev1.Pod,
) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := m.proxyRaw(requestCtx, node.Name, "stats/summary")
	m.recordEndpoint(node.Name, "summary", err)
	if err != nil {
		klog.V(2).InfoS(
			"kubelet summary unavailable", "node", node.Name, "error", err,
		)
		return
	}
	var data summary
	if json.Unmarshal(body, &data) != nil {
		klog.V(2).InfoS("kubelet summary invalid", "node", node.Name)
		return
	}
	if m.featureEnabled(feature.PressureSignals) &&
		(data.Node.CPU.PSI != nil || data.Node.Memory.PSI != nil) {
		psi := maxPSI(data.Node.CPU.PSI, data.Node.Memory.PSI)
		if psi >= m.cfg.PSIWarningPercent {
			reason, severity := constant.ReasonNodePSIHigh, model.SeverityWarning
			if psi >= m.cfg.PSICriticalPercent {
				severity = model.SeverityCritical
			}
			m.observe(node.Name+"/"+reason, true,
				func() {
					m.report(
						node.Name, reason, severity,
						fmt.Sprintf(
							"Node %s pressure stall is %.1f%% over the last 10s",
							node.Name, psi,
						),
					)
				},
				func() {})
		} else {
			m.observe(
				node.Name+"/"+constant.ReasonNodePSIHigh,
				false,
				func() {},
				func() {
					m.resolve(node.Name, constant.ReasonNodePSIHigh)
				},
			)
		}
	}
	if m.featureEnabled(feature.NetworkErrors) && data.Node.Network != nil {
		m.checkNetwork(node.Name, *data.Node.Network)
	}
	if m.featureEnabled(feature.CPUUsage) ||
		m.featureEnabled(feature.MemoryUsage) ||
		m.featureEnabled(feature.StorageUsage) {
		m.checkPodUsage(data.Pods, pods)
	}
}

func (m *Monitor) checkPodUsage(
	summaries []podSummary, pods map[string]*corev1.Pod,
) {
	for _, podSummary := range summaries {
		pod := pods[podSummary.PodRef.Namespace+"/"+podSummary.PodRef.Name]
		if pod == nil {
			continue
		}
		limits := containerLimits(pod)
		for _, container := range podSummary.Containers {
			limit, ok := limits[container.Name]
			if ok {
				m.checkContainerUsage(pod, container, limit)
			}
		}
	}
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

func (m *Monitor) checkContainerUsage(
	pod *corev1.Pod, container containerSummary, limit corev1.ResourceList,
) {
	if m.featureEnabled(feature.MemoryUsage) && container.Memory != nil {
		memoryLimit := limit.Memory()
		if memoryLimit != nil && !memoryLimit.IsZero() {
			percent := float64(container.Memory.WorkingSetBytes) /
				memoryLimit.AsApproximateFloat64() * 100
			m.reportUsage(
				pod, container.Name, percent,
				m.cfg.MemoryWarningPercent, m.cfg.MemoryCriticalPercent,
				constant.ReasonContainerMemoryHigh,
				fmt.Sprintf("%d bytes", container.Memory.WorkingSetBytes),
				memoryLimit.String(), "memory",
			)
		}
	}
	if m.featureEnabled(feature.CPUUsage) && container.CPU != nil {
		if cpuLimit := limit.Cpu(); cpuLimit != nil && !cpuLimit.IsZero() {
			percent := float64(container.CPU.UsageNanoCores) /
				(cpuLimit.AsApproximateFloat64() * 1e9) * 100
			m.reportUsage(
				pod, container.Name, percent,
				m.cfg.CPUWarningPercent, m.cfg.CPUCriticalPercent,
				constant.ReasonContainerCPUHigh,
				fmt.Sprintf("%d nanocores", container.CPU.UsageNanoCores),
				cpuLimit.String(), "cpu",
			)
		}
	}
	if m.featureEnabled(feature.StorageUsage) && container.RootFS != nil {
		storageLimit := limit.StorageEphemeral()
		if storageLimit != nil && !storageLimit.IsZero() {
			percent := float64(container.RootFS.UsedBytes) /
				storageLimit.AsApproximateFloat64() * 100
			m.reportUsage(
				pod, container.Name, percent,
				m.cfg.EphemeralStorageWarningPercent,
				m.cfg.EphemeralStorageCriticalPercent,
				constant.ReasonContainerEphemeralStorageHigh,
				fmt.Sprintf("%d bytes", container.RootFS.UsedBytes),
				storageLimit.String(), "ephemeral-storage",
			)
		}
	}
}

func (m *Monitor) reportUsage(
	pod *corev1.Pod, container string, percent, warning, critical float64,
	reason, usage, limit, unit string,
) {
	owner := pod.Namespace + "/" + pod.Name
	if len(pod.OwnerReferences) == 0 {
		owner = pod.Name
	}
	ev := event.Event{
		Resource: "pod", Namespace: pod.Namespace, PodName: pod.Name,
		PodUID:        string(pod.UID),
		PodLineageID:  pod.Annotations[event.PodLineageAnnotation],
		ContainerName: container, Reason: reason,
	}
	key := correlation.IncidentKey(ev, owner, nil)
	warning, critical = m.adaptiveUsageThreshold(
		usageBaselineKey(pod, container, reason), percent, warning, critical,
	)
	if percent < warning {
		m.observe(
			string(key), false, func() {},
			func() { m.correlator.MarkResolved(key) },
		)
		return
	}
	severity := model.SeverityWarning
	if percent >= critical {
		severity = model.SeverityCritical
	}
	m.observe(string(key), true,
		func() {
			m.correlator.Process(event.Event{
				Resource: "pod", Namespace: pod.Namespace,
				PodName: pod.Name, PodUID: string(pod.UID),
				PodLineageID:  pod.Annotations[event.PodLineageAnnotation],
				ContainerName: container,
				Reason:        reason, Severity: severity,
				Hint: fmt.Sprintf(
					"container %s %s usage is %.0f%% of its limit (%s/%s)",
					container, unit, percent, usage, limit,
				),
			},
				owner, nil)
		}, func() {})
}

func usageBaselineKey(pod *corev1.Pod, container, reason string) string {
	identity := string(pod.UID)
	if identity == "" {
		identity = pod.Namespace + "/" + pod.Name
	}
	return identity + "/" + container + "/" + reason
}

// adaptiveUsageThreshold learns a workload's normal usage from kubelet
// Summary samples. It only raises the warning threshold, requires several
// samples, and never moves it into the critical boundary. Critical usage
// remains governed by configuration, so a busy workload cannot hide an
// imminent limit breach.
func (m *Monitor) adaptiveUsageThreshold(
	key string, percent, warning, critical float64,
) (float64, float64) {
	if warning <= 0 || critical <= warning || percent >= critical {
		return warning, critical
	}
	now := m.now()
	m.mu.Lock()
	baseline := m.baselines[key]
	if baseline.Samples == 0 {
		baseline.Percent = percent
	} else {
		// Welford's online update keeps the observed spread without retaining
		// every sample. This makes the learned threshold follow real workload
		// variability while remaining bounded and ConfigMap-friendly.
		count := float64(baseline.Samples + 1)
		delta := percent - baseline.Percent
		baseline.Percent += delta / count
		baseline.M2 += delta * (percent - baseline.Percent)
	}
	if baseline.Samples < 1000000 {
		baseline.Samples++
	}
	baseline.Updated = now
	m.baselines[key] = baseline
	m.mu.Unlock()
	if baseline.Samples < 5 || baseline.Percent <= warning {
		return warning, critical
	}
	learned := baseline.Percent * 1.25
	if baseline.Samples > 1 && baseline.M2 > 0 {
		deviation := math.Sqrt(baseline.M2 / float64(baseline.Samples-1))
		learned = math.Max(learned, baseline.Percent+2*deviation)
	}
	maxWarning := critical - 1
	if learned > maxWarning {
		learned = maxWarning
	}
	if learned > warning {
		warning = learned
	}
	return warning, critical
}

func maxPSI(stats ...*psiStats) float64 {
	max := 0.0
	for _, stat := range stats {
		if stat == nil {
			continue
		}
		for _, value := range []float64{stat.Some.Avg10, stat.Full.Avg10} {
			if value > max {
				max = value
			}
		}
	}
	return max
}
