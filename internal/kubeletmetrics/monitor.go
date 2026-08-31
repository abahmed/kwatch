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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

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
	failures   map[string]int
	successes  map[string]int
	stateSeen  map[string]time.Time
	baselines  map[string]usageBaseline
	now        func() time.Time
	mu         sync.Mutex
	endpoint   map[string]endpointStatus
	podCache   map[string]*corev1.Pod
	podCacheAt time.Time
	lastSweep  time.Time
	store      StateStore
}

type StateStore interface {
	LoadTelemetryState(context.Context) ([]byte, error)
	SaveTelemetryState(context.Context, []byte) error
}

type persistedState struct {
	Previous  map[string]metricSnapshot `json:"previous"`
	Failures  map[string]int            `json:"failures"`
	Successes map[string]int            `json:"successes"`
	StateSeen map[string]time.Time      `json:"stateSeen"`
	Baselines map[string]usageBaseline  `json:"baselines,omitempty"`
}

type endpointStatus struct {
	Summary, CAdvisor, Runtime bool
	RBACDenied                 bool
}

type Status struct {
	State             string    `json:"state"`
	LastSweep         time.Time `json:"lastSweep"`
	Nodes             int       `json:"nodes"`
	SummaryAvailable  int       `json:"summaryAvailable"`
	CAdvisorAvailable int       `json:"cadvisorAvailable"`
	RuntimeAvailable  int       `json:"runtimeAvailable"`
	RBACDenied        int       `json:"rbacDenied"`
}

type metricSnapshot struct {
	At        time.Time
	Throttled float64
	Periods   float64
	RxErrors  uint64
	TxErrors  uint64
}

type usageBaseline struct {
	Percent float64   `json:"percent"`
	Samples int       `json:"samples"`
	Updated time.Time `json:"updated"`
}

type summary struct {
	Node nodeSummary  `json:"node"`
	Pods []podSummary `json:"pods"`
}

type podSummary struct {
	PodRef     podReference       `json:"podRef"`
	Containers []containerSummary `json:"containers"`
}

type podReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type containerSummary struct {
	Name   string           `json:"name"`
	CPU    *cpuStats        `json:"cpu"`
	Memory *memoryStats     `json:"memory"`
	RootFS *filesystemStats `json:"rootfs"`
}

type cpuStats struct {
	UsageNanoCores uint64 `json:"usageNanoCores"`
}

type memoryStats struct {
	WorkingSetBytes uint64 `json:"workingSetBytes"`
}

type filesystemStats struct {
	UsedBytes uint64 `json:"usedBytes"`
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
	return &Monitor{client: client, cfg: cfg, correlator: correlator, previous: make(map[string]metricSnapshot),
		failures: make(map[string]int), successes: make(map[string]int), stateSeen: make(map[string]time.Time), baselines: make(map[string]usageBaseline), endpoint: make(map[string]endpointStatus), podCache: make(map[string]*corev1.Pod), now: time.Now}
}

func (m *Monitor) Start(ctx context.Context) {
	if !m.cfg.Enabled || m.client == nil {
		return
	}
	m.loadState(ctx)
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
	nodes, err := m.nodes(ctx)
	if err != nil {
		return
	}
	pods := m.pods(ctx)
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range nodes {
		node := &nodes[i]
		wg.Add(1)
		go func(node *corev1.Node) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			m.checkSummary(ctx, node, pods)
			m.checkCadvisor(ctx, node)
			m.checkRuntimeMetrics(ctx, node)
		}(node)
	}
	wg.Wait()
	m.pruneSnapshots(nodes)
	m.pruneSignalState()
	m.mu.Lock()
	m.lastSweep = m.now()
	m.mu.Unlock()
	m.saveState(ctx)
}

func (m *Monitor) Snapshot() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := Status{State: "unavailable", LastSweep: m.lastSweep, Nodes: len(m.endpoint)}
	for _, endpoint := range m.endpoint {
		if endpoint.Summary {
			status.SummaryAvailable++
		}
		if endpoint.CAdvisor {
			status.CAdvisorAvailable++
		}
		if endpoint.Runtime {
			status.RuntimeAvailable++
		}
		if endpoint.RBACDenied {
			status.RBACDenied++
		}
	}
	if status.RBACDenied > 0 && status.SummaryAvailable == 0 {
		status.State = "rbacDenied"
	} else if status.SummaryAvailable > 0 {
		status.State = "healthy"
		if status.SummaryAvailable < status.Nodes || status.CAdvisorAvailable < status.Nodes {
			status.State = "partial"
		}
	}
	return status
}

func (m *Monitor) TelemetryStatus() interface{} {
	return m.Snapshot()
}

func (m *Monitor) SetStateStore(store StateStore) { m.store = store }

func (m *Monitor) loadState(ctx context.Context) {
	if !m.cfg.PersistState || m.store == nil {
		return
	}
	data, err := m.store.LoadTelemetryState(ctx)
	if err != nil || len(data) == 0 {
		return
	}
	var state persistedState
	if json.Unmarshal(data, &state) != nil {
		return
	}
	m.mu.Lock()
	if state.Previous != nil {
		m.previous = state.Previous
	}
	if state.Failures != nil {
		m.failures = state.Failures
	}
	if state.Successes != nil {
		m.successes = state.Successes
	}
	if state.StateSeen != nil {
		m.stateSeen = state.StateSeen
	}
	if state.Baselines != nil {
		m.baselines = state.Baselines
	}
	m.mu.Unlock()
}

func (m *Monitor) saveState(ctx context.Context) {
	if !m.cfg.PersistState || m.store == nil {
		return
	}
	m.mu.Lock()
	state := persistedState{
		Previous:  cloneMetricSnapshots(m.previous),
		Failures:  cloneIntMap(m.failures),
		Successes: cloneIntMap(m.successes),
		StateSeen: cloneTimes(m.stateSeen),
		Baselines: cloneUsageBaselines(m.baselines),
	}
	m.mu.Unlock()
	data, err := json.Marshal(state)
	if err == nil {
		_ = m.store.SaveTelemetryState(ctx, data)
	}
}

func cloneMetricSnapshots(source map[string]metricSnapshot) map[string]metricSnapshot {
	clone := make(map[string]metricSnapshot, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneUsageBaselines(source map[string]usageBaseline) map[string]usageBaseline {
	clone := make(map[string]usageBaseline, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneIntMap(source map[string]int) map[string]int {
	clone := make(map[string]int, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func cloneTimes(source map[string]time.Time) map[string]time.Time {
	clone := make(map[string]time.Time, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func (m *Monitor) recordEndpoint(node, endpoint string, err error) {
	m.mu.Lock()
	status := m.endpoint[node]
	available := err == nil
	denied := err != nil && apierrors.IsForbidden(err)
	switch endpoint {
	case "summary":
		status.Summary = available
	case "cadvisor":
		status.CAdvisor = available
	case "runtime":
		status.Runtime = available
	}
	status.RBACDenied = status.RBACDenied || denied
	m.endpoint[node] = status
	m.mu.Unlock()
}

func (m *Monitor) pruneSnapshots(nodes []corev1.Node) {
	active := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		active[node.Name] = struct{}{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.previous {
		node, ok := snapshotNode(key)
		if ok {
			if _, exists := active[node]; !exists {
				delete(m.previous, key)
			}
		}
	}
	for node := range m.endpoint {
		if _, exists := active[node]; !exists {
			delete(m.endpoint, node)
		}
	}
}

func snapshotNode(key string) (string, bool) {
	parts := strings.SplitN(key, "/", 3)
	if len(parts) < 2 {
		return "", false
	}
	if parts[0] == "network" || parts[0] == "runtime" {
		return parts[1], true
	}
	if parts[0] == "cpu" && len(parts) == 3 {
		return parts[1], true
	}
	return "", false
}

func (m *Monitor) pods(ctx context.Context) map[string]*corev1.Pod {
	now := m.now()
	m.mu.Lock()
	if now.Sub(m.podCacheAt) < 15*time.Second && len(m.podCache) > 0 {
		cached := m.podCache
		m.mu.Unlock()
		return cached
	}
	m.mu.Unlock()
	result := make(map[string]*corev1.Pod)
	continueToken := ""
	for {
		pods, err := m.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{Limit: 500, Continue: continueToken})
		if err != nil {
			return nil
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			result[pod.Namespace+"/"+pod.Name] = pod
		}
		continueToken = pods.Continue
		if continueToken == "" {
			break
		}
	}
	m.mu.Lock()
	m.podCache, m.podCacheAt = result, now
	m.mu.Unlock()
	return result
}

func (m *Monitor) nodes(ctx context.Context) ([]corev1.Node, error) {
	var result []corev1.Node
	continueToken := ""
	for {
		nodes, err := m.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 500, Continue: continueToken})
		if err != nil {
			return nil, err
		}
		result = append(result, nodes.Items...)
		continueToken = nodes.Continue
		if continueToken == "" {
			return result, nil
		}
	}
}

func (m *Monitor) checkSummary(ctx context.Context, node *corev1.Node, pods map[string]*corev1.Pod) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := m.proxyRaw(requestCtx, node.Name, "stats/summary")
	m.recordEndpoint(node.Name, "summary", err)
	if err != nil {
		klog.V(2).InfoS("kubelet summary unavailable", "node", node.Name, "error", err)
		return
	}
	var data summary
	if json.Unmarshal(body, &data) != nil {
		klog.V(2).InfoS("kubelet summary invalid", "node", node.Name)
		return
	}
	if data.Node.CPU.PSI != nil || data.Node.Memory.PSI != nil {
		psi := maxPSI(data.Node.CPU.PSI, data.Node.Memory.PSI)
		if psi >= m.cfg.PSIWarningPercent {
			reason, severity := constant.ReasonNodePSIHigh, model.SeverityWarning
			if psi >= m.cfg.PSICriticalPercent {
				severity = model.SeverityCritical
			}
			m.observe(node.Name+"/"+reason, true,
				func() {
					m.report(node.Name, reason, severity, fmt.Sprintf("Node %s pressure stall is %.1f%% over the last 10s", node.Name, psi))
				},
				func() {})
		} else {
			m.observe(node.Name+"/"+constant.ReasonNodePSIHigh, false, func() {}, func() { m.resolve(node.Name, constant.ReasonNodePSIHigh) })
		}
	}
	if data.Node.Network != nil {
		m.checkNetwork(node.Name, *data.Node.Network)
	}
	m.checkPodUsage(data.Pods, pods)
}

func (m *Monitor) checkPodUsage(summaries []podSummary, pods map[string]*corev1.Pod) {
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
	for _, container := range append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...) {
		limits[container.Name] = container.Resources.Limits
	}
	return limits
}

func (m *Monitor) checkContainerUsage(pod *corev1.Pod, container containerSummary, limit corev1.ResourceList) {
	if container.Memory != nil {
		if memoryLimit := limit.Memory(); memoryLimit != nil && !memoryLimit.IsZero() {
			percent := float64(container.Memory.WorkingSetBytes) / memoryLimit.AsApproximateFloat64() * 100
			m.reportUsage(pod, container.Name, percent, m.cfg.MemoryWarningPercent, m.cfg.MemoryCriticalPercent,
				constant.ReasonContainerMemoryHigh, fmt.Sprintf("%d bytes", container.Memory.WorkingSetBytes), memoryLimit.String(), "memory")
		}
	}
	if container.CPU != nil {
		if cpuLimit := limit.Cpu(); cpuLimit != nil && !cpuLimit.IsZero() {
			percent := float64(container.CPU.UsageNanoCores) / (cpuLimit.AsApproximateFloat64() * 1e9) * 100
			m.reportUsage(pod, container.Name, percent, m.cfg.CPUWarningPercent, m.cfg.CPUCriticalPercent,
				constant.ReasonContainerCPUHigh, fmt.Sprintf("%d nanocores", container.CPU.UsageNanoCores), cpuLimit.String(), "cpu")
		}
	}
	if container.RootFS != nil {
		if storageLimit := limit.StorageEphemeral(); storageLimit != nil && !storageLimit.IsZero() {
			percent := float64(container.RootFS.UsedBytes) / storageLimit.AsApproximateFloat64() * 100
			m.reportUsage(pod, container.Name, percent, m.cfg.EphemeralStorageWarningPercent, m.cfg.EphemeralStorageCriticalPercent,
				constant.ReasonContainerEphemeralStorageHigh, fmt.Sprintf("%d bytes", container.RootFS.UsedBytes), storageLimit.String(), "ephemeral-storage")
		}
	}
}

func (m *Monitor) reportUsage(pod *corev1.Pod, container string, percent, warning, critical float64, reason, usage, limit, unit string) {
	key := correlation.BuildKey(pod.Namespace, pod.Namespace+"/"+pod.Name, reason, container)
	warning, critical = m.adaptiveUsageThreshold(string(key), percent, warning, critical)
	if percent < warning {
		m.observe(string(key), false, func() {}, func() { m.correlator.MarkResolved(key) })
		return
	}
	severity := model.SeverityWarning
	if percent >= critical {
		severity = model.SeverityCritical
	}
	m.observe(string(key), true,
		func() {
			m.correlator.Process(event.Event{Resource: "pod", Namespace: pod.Namespace, PodName: pod.Name, ContainerName: container,
				Reason: reason, OwnerKind: "Pod", Severity: severity,
				Hint: fmt.Sprintf("container %s %s usage is %.0f%% of its limit (%s/%s)", container, unit, percent, usage, limit)},
				pod.Namespace+"/"+pod.Name, nil)
		}, func() {})
}

// adaptiveUsageThreshold learns a workload's normal usage from kubelet
// Summary samples. It only raises the warning threshold, requires several
// samples, and never moves it into the critical boundary. Critical usage
// remains governed by configuration, so a busy workload cannot hide an
// imminent limit breach.
func (m *Monitor) adaptiveUsageThreshold(key string, percent, warning, critical float64) (float64, float64) {
	if warning <= 0 || critical <= warning || percent >= critical {
		return warning, critical
	}
	now := m.now()
	m.mu.Lock()
	baseline := m.baselines[key]
	if baseline.Samples == 0 {
		baseline.Percent = percent
	} else {
		baseline.Percent = baseline.Percent*0.8 + percent*0.2
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
		m.observe(key, true,
			func() {
				m.report(node, reason, severity, fmt.Sprintf("Node %s network errors increased at %.2f errors/sec", node, rate))
			},
			func() {})
	} else {
		m.observe(key, false, func() {}, func() { m.resolve(node, constant.ReasonNodeNetworkErrors) })
	}
}

func (m *Monitor) checkCadvisor(ctx context.Context, node *corev1.Node) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := m.proxyRaw(requestCtx, node.Name, "metrics/cadvisor")
	m.recordEndpoint(node.Name, "cadvisor", err)
	if err != nil {
		klog.V(2).InfoS("kubelet cAdvisor metrics unavailable", "node", node.Name, "error", err)
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
			incidentKey := "cpu/" + node.Name + "/" + key
			m.observe(incidentKey, true,
				func() { m.reportContainer(namespace, pod, container, owner, severity, percent) },
				func() {})
		} else if namespace != "" && pod != "" {
			incidentKey := "cpu/" + node.Name + "/" + key
			m.observe(incidentKey, false, func() {}, func() { m.resolveContainer(namespace, pod, container) })
		}
	}
}

func (m *Monitor) checkRuntimeMetrics(ctx context.Context, node *corev1.Node) {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, err := m.proxyRaw(requestCtx, node.Name, "metrics")
	m.recordEndpoint(node.Name, "runtime", err)
	if err != nil {
		klog.V(2).InfoS("kubelet metrics unavailable", "node", node.Name, "error", err)
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
		m.observe(key, true,
			func() {
				m.report(node.Name, reason, severity, fmt.Sprintf("Node %s kubelet runtime errors increased at %.2f errors/sec", node.Name, rate))
			},
			func() {})
	} else {
		m.observe(key, false, func() {}, func() { m.resolve(node.Name, constant.ReasonNodeRuntimeErrors) })
	}
}

func (m *Monitor) proxyRaw(ctx context.Context, node, suffix string) ([]byte, error) {
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
		body, err := m.client.CoreV1().RESTClient().Get().Resource("nodes").Name(node).
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
	value, err := strconv.ParseFloat(parts[1], 64)
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
