package kubeletmetrics

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/model"
)

func (m *Monitor) report(
	node, reason string, severity model.Severity, hint string,
) {
	m.correlator.Process(
		event.Event{
			Resource: "node", NodeName: node, PodName: node,
			Reason: reason, Hint: hint, Severity: severity,
		},
		node, nil,
	)
}

func (m *Monitor) reportContainer(
	pod *corev1.Pod, container, owner string,
	severity model.Severity, percent float64,
) {
	ev := event.Event{
		Resource: "pod", ContainerName: container,
		Reason: constant.ReasonContainerCPUThrottled, Severity: severity,
		Hint: fmt.Sprintf(
			"container %s CPU throttling is %.1f%% of scheduling periods",
			container, percent,
		),
	}
	if pod != nil {
		ev.Namespace, ev.PodName = pod.Namespace, pod.Name
		ev.PodUID = string(pod.UID)
		ev.PodLineageID = pod.Annotations[event.PodLineageAnnotation]
		if len(pod.OwnerReferences) == 0 {
			owner = pod.Name
		}
	}
	m.correlator.Process(ev, owner, nil)
}

func (m *Monitor) resolve(node, reason string) {
	m.correlator.MarkResolved(correlation.BuildKey("", node, reason, ""))
}

func (m *Monitor) resolveContainer(namespace, pod, container string) {
	obj := m.cachedPod(namespace, pod)
	owner := namespace + "/" + pod
	if obj != nil && len(obj.OwnerReferences) == 0 {
		owner = pod
	}
	ev := event.Event{
		Resource: "pod", Namespace: namespace, PodName: pod,
		ContainerName: container,
		Reason:        constant.ReasonContainerCPUThrottled,
	}
	if obj != nil {
		ev.PodUID = string(obj.UID)
		ev.PodLineageID = obj.Annotations[event.PodLineageAnnotation]
	}
	m.correlator.MarkResolved(correlation.IncidentKey(ev, owner, nil))
}

func (m *Monitor) cachedPod(namespace, name string) *corev1.Pod {
	m.mu.Lock()
	defer m.mu.Unlock()
	pod := m.podCache[namespace+"/"+name]
	if pod == nil {
		return nil
	}
	return pod.DeepCopy()
}

func (m *Monitor) observe(key string, failing bool, report, resolve func()) {
	failureThreshold := m.cfg.FailureThreshold
	if failureThreshold <= 0 {
		failureThreshold = 1
	}
	recoveryThreshold := m.cfg.RecoveryThreshold
	if recoveryThreshold <= 0 {
		recoveryThreshold = 1
	}
	m.mu.Lock()
	m.stateSeen[key] = m.now()
	if failing {
		m.failures[key]++
		m.successes[key] = 0
		reportNow := m.failures[key] >= failureThreshold
		m.mu.Unlock()
		if reportNow {
			report()
		}
		return
	}
	m.successes[key]++
	m.failures[key] = 0
	resolveNow := m.successes[key] >= recoveryThreshold
	if resolveNow {
		delete(m.successes, key)
	}
	m.mu.Unlock()
	if resolveNow {
		resolve()
	}
}

func (m *Monitor) pruneSignalState() {
	interval := time.Duration(m.cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	cutoff := m.now().Add(-10 * interval)
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, seen := range m.stateSeen {
		if seen.Before(cutoff) {
			delete(m.stateSeen, key)
			delete(m.failures, key)
			delete(m.successes, key)
		}
	}
	for key, baseline := range m.baselines {
		if baseline.Updated.Before(cutoff) {
			delete(m.baselines, key)
		}
	}
}

func (m *Monitor) summaryFeaturesEnabled() bool {
	return m.featureEnabled(feature.PressureSignals) ||
		m.featureEnabled(feature.NetworkErrors) ||
		m.featureEnabled(feature.CPUUsage) ||
		m.featureEnabled(feature.MemoryUsage) ||
		m.featureEnabled(feature.StorageUsage)
}

func (m *Monitor) featureEnabled(id feature.ID) bool {
	// A zero plan is retained as compatibility mode for callers that construct
	// a Monitor directly. The application always supplies a complete plan.
	if len(m.plan.Decisions) == 0 {
		return true
	}
	return m.plan.Enabled(id)
}
