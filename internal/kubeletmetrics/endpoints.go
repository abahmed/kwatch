package kubeletmetrics

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func (m *Monitor) checkNetwork(node string, current networkStats) {
	now := m.now()
	key := "network/" + node
	m.mu.Lock()
	previous, exists := m.previous[key]
	m.previous[key] = metricSnapshot{
		At: now, RxErrors: current.RxErrors, TxErrors: current.TxErrors,
	}
	m.mu.Unlock()
	if !exists || !now.After(previous.At) {
		return
	}
	if current.RxErrors < previous.RxErrors ||
		current.TxErrors < previous.TxErrors {
		return
	}
	seconds := now.Sub(previous.At).Seconds()
	rate := float64(
		(current.RxErrors-previous.RxErrors)+
			(current.TxErrors-previous.TxErrors),
	) / seconds
	if rate >= m.cfg.NetworkErrorRateWarning {
		reason, severity := constant.ReasonNodeNetworkErrors, model.SeverityWarning
		if rate >= m.cfg.NetworkErrorRateCritical {
			severity = model.SeverityCritical
		}
		m.observe(key, true,
			func() {
				m.report(
					node, reason, severity,
					fmt.Sprintf(
						"Node %s network errors increased at %.2f errors/sec",
						node, rate,
					),
				)
			},
			func() {})
	} else {
		m.observe(
			key, false, func() {},
			func() { m.resolve(node, constant.ReasonNodeNetworkErrors) },
		)
	}
}

func (m *Monitor) checkCadvisor(ctx context.Context, node *corev1.Node) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := m.proxyRaw(requestCtx, node.Name, "metrics/cadvisor")
	m.recordEndpoint(node.Name, "cadvisor", err)
	if err != nil {
		klog.V(2).InfoS(
			"kubelet cAdvisor metrics unavailable",
			"node", node.Name, "error", err,
		)
		return
	}
	metrics := parseCounters(body)
	now := m.now()
	for key, current := range metrics {
		if current.Periods <= 0 {
			continue
		}
		m.mu.Lock()
		previous, exists := m.previous["cpu/"+node.Name+"/"+key]
		m.previous["cpu/"+node.Name+"/"+key] = metricSnapshot{
			At: now, Throttled: current.Throttled,
			Periods: current.Periods,
		}
		m.mu.Unlock()
		if !exists || current.Throttled < previous.Throttled ||
			current.Periods <= previous.Periods {
			continue
		}
		percent := (current.Throttled - previous.Throttled) /
			(current.Periods - previous.Periods) * 100
		namespace, pod, container := metricIdentity(key)
		if namespace == "" || pod == "" || container == "" {
			continue
		}
		owner := namespace + "/" + pod
		if percent >= m.cfg.CPUThrottlingWarningPercent {
			severity := model.SeverityWarning
			if percent >= m.cfg.CPUThrottlingCriticalPercent {
				severity = model.SeverityCritical
			}
			incidentKey := "cpu/" + node.Name + "/" + key
			m.observe(incidentKey, true,
				func() {
					m.reportContainer(
						m.cachedPod(namespace, pod), container, owner,
						severity, percent,
					)
				},
				func() {})
		} else if namespace != "" && pod != "" {
			incidentKey := "cpu/" + node.Name + "/" + key
			m.observe(
				incidentKey, false, func() {},
				func() { m.resolveContainer(namespace, pod, container) },
			)
		}
	}
}

func (m *Monitor) checkRuntimeMetrics(ctx context.Context, node *corev1.Node) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := m.proxyRaw(requestCtx, node.Name, "metrics")
	m.recordEndpoint(node.Name, "runtime", err)
	if err != nil {
		klog.V(2).InfoS(
			"kubelet metrics unavailable", "node", node.Name, "error", err,
		)
		return
	}
	current := sumCounters(
		parseNamedCounters(body, "kubelet_runtime_operations_errors_total"),
	)
	now := m.now()
	key := "runtime/" + node.Name
	m.mu.Lock()
	previous, exists := m.previous[key]
	m.previous[key] = metricSnapshot{At: now, Throttled: current}
	m.mu.Unlock()
	if !exists || current <= 0 || current < previous.Throttled ||
		!now.After(previous.At) {
		return
	}
	seconds := now.Sub(previous.At).Seconds()
	if seconds <= 0 {
		return
	}
	rate := (current - previous.Throttled) / seconds
	if rate >= m.cfg.RuntimeErrorRateWarning {
		reason, severity := constant.ReasonNodeRuntimeErrors, model.SeverityWarning
		if rate >= m.cfg.RuntimeErrorRateCritical {
			severity = model.SeverityCritical
		}
		m.observe(key, true,
			func() {
				m.report(
					node.Name, reason, severity,
					fmt.Sprintf(
						"Node %s kubelet runtime errors increased at %.2f errors/sec",
						node.Name, rate,
					),
				)
			},
			func() {})
	} else {
		m.observe(
			key, false, func() {},
			func() { m.resolve(node.Name, constant.ReasonNodeRuntimeErrors) },
		)
	}
}

func (m *Monitor) proxyRaw(
	ctx context.Context, node, suffix string,
) ([]byte, error) {
	delays := []time.Duration{0, 100 * time.Millisecond, 250 * time.Millisecond}
	var lastErr error
	for attempt, delay := range delays {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		body, err := m.client.CoreV1().RESTClient().Get().Resource("nodes").
			Name(node).
			SubResource("proxy").Suffix(suffix).DoRaw(ctx)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == len(delays)-1 {
			break
		}
	}
	return nil, lastErr
}
