package kubeletmetrics

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (m *Monitor) report(
	node, reason string, severity model.Severity, hint string,
) {
	m.correlator.Process(
		observe.NodeNamed(node, reason).
			WithSeverity(severity).WithHint(hint),
	)
}

func (m *Monitor) reportContainer(
	pod *corev1.Pod, container string, owner model.ObjectRef,
	severity model.Severity, percent float64,
) {
	m.correlator.Process(
		observe.PodOwnedBy(
			pod, container, constant.ReasonContainerCPUThrottled, owner,
		).WithSeverity(severity).WithHint(fmt.Sprintf(
			"container %s CPU throttling is %.1f%% of scheduling periods",
			container, percent,
		)),
	)
}

func (m *Monitor) resolve(node, reason string) {
	m.correlator.Resolve(
		model.ObjectRef{Kind: "node", Name: node}, reason,
	)
}

func (m *Monitor) resolveContainer(namespace, pod, container string) {
	obj := m.cachedPod(namespace, pod)
	owner := m.podOwner(obj, namespace, pod)
	// The recovery is described with the same observation the failure was,
	// so the two cannot disagree about which incident they mean.
	obs := observe.PodOwnedBy(
		obj, container, constant.ReasonContainerCPUThrottled, owner,
	)
	if obj == nil {
		obs.Subject.Namespace, obs.Subject.Name = namespace, pod
	}
	m.correlator.ResolveObserved(obs)
}

// podOwner returns the incident owner for a pod the way the rest of the
// pipeline keys pod incidents: the owning workload (ReplicaSet resolved to its
// Deployment) when a resolver is wired, the pod itself when it has no owner,
// and "namespace/pod" only as the last resort. Keying by pod instead produced
// one alert per replica and let three replicas of one Deployment pass for a
// "mass failure" on their own ServiceAccount.
func (m *Monitor) podOwner(
	pod *corev1.Pod, namespace, name string,
) model.ObjectRef {
	if pod != nil {
		if m.owners != nil {
			if owner := m.owners.OwnerOf(pod); owner.Name != "" {
				return owner
			}
		}
		if len(pod.OwnerReferences) == 0 {
			return observe.SelfOwner("Pod", pod.Namespace, pod.Name)
		}
	}
	// Last resort: the pod is not in the cache, so its workload cannot be
	// read. The slash-joined name is the key this monitor has always used
	// here; incidents already open under it must keep resolving. The owner
	// carries no Kind on purpose -- it owns the incident, but claiming "Pod"
	// would print a workload kind kwatch never actually established, and a
	// pod that is missing from the cache is exactly the case where it may
	// well have had one.
	return model.ObjectRef{Name: namespace + "/" + name}
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
		if reportNow {
			m.failing[key] = true
		}
		m.mu.Unlock()
		if reportNow {
			report()
		}
		return
	}
	if !m.failing[key] {
		// This signal was never reported failing, so there is nothing to
		// resolve. Resolving anyway ran a locked engine pass per healthy
		// signal per tick -- and every node reports several of them.
		delete(m.successes, key)
		delete(m.failures, key)
		m.mu.Unlock()
		return
	}
	m.successes[key]++
	m.failures[key] = 0
	resolveNow := m.successes[key] >= recoveryThreshold
	if resolveNow {
		delete(m.successes, key)
		delete(m.failing, key)
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
			delete(m.failing, key)
		}
	}
	for key, baseline := range m.baselines {
		if baseline.Updated.Before(cutoff) {
			delete(m.baselines, key)
		}
	}
}
