package controlplane

import (
	"context"
	"fmt"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

func (m *Monitor) check(ctx context.Context) {
	probeCtx, cancel := context.WithTimeout(ctx, defaultProbeTimeout)
	defer cancel()
	m.mu.Lock()
	m.status.LastCheck = m.nowTime()
	m.mu.Unlock()
	m.checkAPIServer(probeCtx)
	m.checkCoreDNS(probeCtx)
	pods, err := m.componentPods(probeCtx)
	if err != nil {
		m.markComponentsUnavailable(err)
		m.recordProbeError("control-plane pod discovery", err)
		return
	}
	for _, component := range controlPlaneComponents {
		m.checkComponent(probeCtx, component, pods)
	}
	m.mu.Lock()
	m.status.LastCheck = m.nowTime()
	m.mu.Unlock()
}

func (m *Monitor) markComponentsUnavailable(err error) {
	checked := m.nowTime()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Components == nil {
		m.status.Components = make(map[string]EndpointStatus)
	}
	for _, component := range controlPlaneComponents {
		m.status.Components[component] = EndpointStatus{
			Name: component, Available: false, LastError: safeProbeError(err),
			LastChecked: checked, Supported: true,
		}
	}
}

func (m *Monitor) checkCoreDNS(ctx context.Context) {
	started := m.nowTime()
	m.mu.RLock()
	resolver := m.resolver
	m.mu.RUnlock()
	var err error
	if resolver == nil {
		err = fmt.Errorf("DNS resolver is not configured")
	} else {
		_, err = resolver.LookupHost(ctx, "kubernetes.default.svc")
	}
	checked := m.nowTime()
	status := EndpointStatus{
		Name: "coredns/kubernetes.default.svc", Latency: checked.Sub(started),
		LastChecked: checked, Supported: true, Available: err == nil,
	}
	if err != nil {
		status.LastError = safeProbeError(err)
	}
	m.mu.Lock()
	m.status.CoreDNS = status
	m.mu.Unlock()
	if err != nil {
		m.observe("coredns", false, constant.ReasonCoreDNSUnavailable,
			fmt.Sprintf("DNS lookup kubernetes.default.svc failed: %v", err))
		return
	}
	m.observe("coredns", true, constant.ReasonCoreDNSUnavailable,
		"CoreDNS lookup recovered")
}

func (m *Monitor) checkAPIServer(ctx context.Context) {
	started := m.nowTime()
	_, err := m.restClient.Get().AbsPath("/readyz").Param(
		"verbose", "true").Do(ctx).Raw()
	checked := m.nowTime()
	status := EndpointStatus{
		Name: "kube-apiserver/readyz", Latency: checked.Sub(started),
		LastChecked: checked, Supported: true, Available: err == nil,
	}
	if err != nil {
		status.LastError = safeProbeError(err)
	}
	m.mu.Lock()
	m.status.APIServer = status
	m.mu.Unlock()
	metrics.DefaultRegistry().APIServerLatencyMs.Store(
		status.Latency.Milliseconds(),
	)
	if err != nil {
		metrics.DefaultRegistry().APIServerProbeErrors.Add(1)
		m.observe("api-server", false, constant.ReasonAPIServerUnavailable,
			fmt.Sprintf("kube-apiserver /readyz failed: %v", err))
		return
	}
	threshold := time.Duration(m.cfg.APIServerLatencyWarningMs) * time.Millisecond
	if threshold <= 0 {
		threshold = time.Second
	}
	if status.Latency >= threshold {
		m.observe("api-server-latency", false, constant.ReasonAPIServerLatency,
			fmt.Sprintf("kube-apiserver /readyz took %s (threshold %s)",
				status.Latency.Round(time.Millisecond), threshold))
		return
	}
	m.observe("api-server", true, constant.ReasonAPIServerUnavailable,
		"kube-apiserver /readyz recovered")
	m.observe("api-server-latency", true, constant.ReasonAPIServerLatency,
		"kube-apiserver /readyz latency recovered")
}

func (m *Monitor) recordProbeError(name string, err error) {
	m.mu.Lock()
	m.status.ProbeErrors++
	m.mu.Unlock()
	metrics.DefaultRegistry().ControlPlaneProbeErrors.Add(1)
	klog.ErrorS(err, "controlplane probe failed", "probe", name)
}

func (m *Monitor) observe(key string, healthy bool, reason, hint string) {
	threshold := m.cfg.FailureThreshold
	if threshold <= 0 {
		threshold = defaultFailureSamples
	}
	recovery := m.cfg.RecoveryThreshold
	if recovery <= 0 {
		recovery = defaultRecoverySamples
	}
	m.mu.Lock()
	if healthy {
		if !m.failing[key] {
			delete(m.recoveries, key)
			delete(m.failures, key)
			m.mu.Unlock()
			return
		}
		m.recoveries[key]++
		m.failures[key] = 0
	} else {
		m.failures[key]++
		m.recoveries[key] = 0
	}
	failed := m.failures[key] >= threshold
	resolved := healthy && m.recoveries[key] >= recovery
	if !healthy && failed {
		m.failing[key] = true
	}
	if resolved {
		delete(m.failing, key)
		delete(m.recoveries, key)
	}
	m.mu.Unlock()
	if !healthy && failed {
		obs := observe.Synthetic("controlplane", key, reason).
			WithSeverity(model.SeverityHigh).WithHint(hint)
		obs.Subject.Name = ""
		m.incidentSink.Process(obs)
	}
	if resolved {
		m.incidentSink.Resolve(
			model.ObjectRef{Kind: "controlplane", Name: key}, reason,
		)
	}
}
