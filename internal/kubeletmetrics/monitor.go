package kubeletmetrics

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

type Monitor struct {
	client     kubernetes.Interface
	correlator *correlation.Engine
	cfg        config.KubeletTelemetryMonitor
	previous   map[string]metricSnapshot
	now        func() time.Time
	mu         sync.Mutex
}

type metricSnapshot struct {
	At        time.Time
	Throttled float64
	Periods   float64
	RxErrors  uint64
	TxErrors  uint64
}

type summary struct {
	Node nodeSummary `json:"node"`
}

type nodeSummary struct {
	CPU     resourceStats `json:"cpu"`
	Memory  resourceStats `json:"memory"`
	Network *networkStats `json:"network"`
}

type resourceStats struct {
	PSI *psiStats `json:"psi"`
}

type psiStats struct {
	Some psiWindow `json:"some"`
	Full psiWindow `json:"full"`
}

type psiWindow struct {
	Avg10  float64 `json:"avg10"`
	Avg60  float64 `json:"avg60"`
	Avg300 float64 `json:"avg300"`
}

type networkStats struct {
	RxErrors uint64 `json:"rxErrors"`
	TxErrors uint64 `json:"txErrors"`
}

func New(client kubernetes.Interface, cfg config.KubeletTelemetryMonitor, correlator *correlation.Engine) *Monitor {
	return &Monitor{client: client, cfg: cfg, correlator: correlator, previous: make(map[string]metricSnapshot), now: time.Now}
}

func (m *Monitor) Start(ctx context.Context) {
	if !m.cfg.Enabled || m.client == nil {
		return
	}
	interval := time.Duration(m.cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	m.sweep(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sweep(ctx)
		}
	}
}

func (m *Monitor) sweep(ctx context.Context) {
	nodes, err := m.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	for i := range nodes.Items {
		node := &nodes.Items[i]
		m.checkSummary(ctx, node)
		m.checkCadvisor(ctx, node)
		m.checkRuntimeMetrics(ctx, node)
	}
}

func (m *Monitor) checkSummary(ctx context.Context, node *corev1.Node) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := m.client.CoreV1().RESTClient().Get().Resource("nodes").Name(node.Name).
		SubResource("proxy").Suffix("stats/summary").DoRaw(requestCtx)
	if err != nil {
		return
	}
	var data summary
	if json.Unmarshal(body, &data) != nil {
		return
	}
	if data.Node.CPU.PSI != nil || data.Node.Memory.PSI != nil {
		psi := maxPSI(data.Node.CPU.PSI, data.Node.Memory.PSI)
		if psi >= m.cfg.PSIWarningPercent {
			reason, severity := constant.ReasonNodePSIHigh, model.SeverityWarning
			if psi >= m.cfg.PSICriticalPercent {
				severity = model.SeverityCritical
			}
			m.report(node.Name, reason, severity, fmt.Sprintf("Node %s pressure stall is %.1f%% over the last 10s", node.Name, psi))
		} else {
			m.resolve(node.Name, constant.ReasonNodePSIHigh)
		}
	}
	if data.Node.Network != nil {
		m.checkNetwork(node.Name, *data.Node.Network)
	}
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

func (m *Monitor) checkNetwork(node string, current networkStats) {
	now := m.now()
	key := "network/" + node
	m.mu.Lock()
	previous, exists := m.previous[key]
	m.previous[key] = metricSnapshot{At: now, RxErrors: current.RxErrors, TxErrors: current.TxErrors}
	m.mu.Unlock()
	if !exists || !now.After(previous.At) {
		return
	}
	if current.RxErrors < previous.RxErrors || current.TxErrors < previous.TxErrors {
		return
	}
	seconds := now.Sub(previous.At).Seconds()
	rate := float64((current.RxErrors-previous.RxErrors)+(current.TxErrors-previous.TxErrors)) / seconds
	if rate >= m.cfg.NetworkErrorRateWarning {
		reason, severity := constant.ReasonNodeNetworkErrors, model.SeverityWarning
		if rate >= m.cfg.NetworkErrorRateCritical {
			severity = model.SeverityCritical
		}
		m.report(node, reason, severity, fmt.Sprintf("Node %s network errors increased at %.2f errors/sec", node, rate))
	} else {
		m.resolve(node, constant.ReasonNodeNetworkErrors)
	}
}

func (m *Monitor) checkCadvisor(ctx context.Context, node *corev1.Node) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := m.client.CoreV1().RESTClient().Get().Resource("nodes").Name(node.Name).
		SubResource("proxy").Suffix("metrics/cadvisor").DoRaw(requestCtx)
	if err != nil {
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
		m.previous["cpu/"+node.Name+"/"+key] = metricSnapshot{At: now, Throttled: current.Throttled, Periods: current.Periods}
		m.mu.Unlock()
		if !exists || current.Throttled < previous.Throttled || current.Periods <= previous.Periods {
			continue
		}
		percent := (current.Throttled - previous.Throttled) / (current.Periods - previous.Periods) * 100
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
			m.reportContainer(namespace, pod, container, owner, severity, percent)
		} else if namespace != "" && pod != "" {
			m.resolveContainer(namespace, pod, container)
		}
	}
}

func (m *Monitor) checkRuntimeMetrics(ctx context.Context, node *corev1.Node) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := m.client.CoreV1().RESTClient().Get().Resource("nodes").Name(node.Name).
		SubResource("proxy").Suffix("metrics").DoRaw(requestCtx)
	if err != nil {
		return
	}
	current := sumCounters(parseNamedCounters(body, "kubelet_runtime_operations_errors_total"))
	now := m.now()
	key := "runtime/" + node.Name
	m.mu.Lock()
	previous, exists := m.previous[key]
	m.previous[key] = metricSnapshot{At: now, Throttled: current}
	m.mu.Unlock()
	if !exists || current <= 0 || current < previous.Throttled || !now.After(previous.At) {
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
		m.report(node.Name, reason, severity, fmt.Sprintf("Node %s kubelet runtime errors increased at %.2f errors/sec", node.Name, rate))
	} else {
		m.resolve(node.Name, constant.ReasonNodeRuntimeErrors)
	}
}

type counterPair struct{ Throttled, Periods float64 }

func parseCounters(body []byte) map[string]counterPair {
	out := make(map[string]counterPair)
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, labels, value, ok := metricLine(line)
		if !ok || (name != "container_cpu_cfs_throttled_periods_total" && name != "container_cpu_cfs_periods_total") {
			continue
		}
		key := labels["namespace"] + "/" + labels["pod"] + "/" + labels["container"]
		pair := out[key]
		if name == "container_cpu_cfs_throttled_periods_total" {
			pair.Throttled = value
		} else {
			pair.Periods = value
		}
		out[key] = pair
	}
	return out
}

func parseNamedCounters(body []byte, wanted string) map[string]float64 {
	out := make(map[string]float64)
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		name, labels, value, ok := metricLine(scanner.Text())
		if ok && name == wanted {
			out[labels["operation_type"]] += value
		}
	}
	return out
}

func sumCounters(values map[string]float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}

func metricLine(line string) (string, map[string]string, float64, bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", nil, 0, false
	}
	value, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil {
		return "", nil, 0, false
	}
	name, labels := parts[0], map[string]string{}
	if open := strings.IndexByte(name, '{'); open >= 0 {
		labelText := strings.TrimSuffix(name[open+1:], "}")
		name = name[:open]
		for _, item := range strings.Split(labelText, ",") {
			pair := strings.SplitN(item, "=", 2)
			if len(pair) == 2 {
				labels[pair[0]] = strings.Trim(pair[1], "\"")
			}
		}
	}
	if labels["namespace"] == "" {
		labels["namespace"] = labels["pod_namespace"]
	}
	if labels["pod"] == "" {
		labels["pod"] = labels["pod_name"]
	}
	if labels["container"] == "" {
		labels["container"] = labels["container_name"]
	}
	return name, labels, value, true
}

func metricIdentity(key string) (string, string, string) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return "", "", ""
	}
	return parts[0], parts[1], parts[2]
}

func (m *Monitor) report(node, reason string, severity model.Severity, hint string) {
	m.correlator.Process(event.Event{Resource: "node", NodeName: node, PodName: node, Reason: reason, Hint: hint, Severity: severity}, node, nil)
}

func (m *Monitor) reportContainer(namespace, pod, container, owner string, severity model.Severity, percent float64) {
	m.correlator.Process(event.Event{Resource: "pod", Namespace: namespace, PodName: pod, ContainerName: container, Reason: constant.ReasonContainerCPUThrottled, Severity: severity,
		Hint: fmt.Sprintf("container %s CPU throttling is %.1f%% of scheduling periods", container, percent)}, owner, nil)
}

func (m *Monitor) resolve(node, reason string) {
	m.correlator.MarkResolved(correlation.BuildKey("", node, reason, ""))
}

func (m *Monitor) resolveContainer(namespace, pod, container string) {
	m.correlator.MarkResolved(correlation.BuildKey(namespace, namespace+"/"+pod, constant.ReasonContainerCPUThrottled, container))
}
