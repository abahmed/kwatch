package kubeletmetrics

import (
	"context"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
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
	m.resetEndpointStatus(nodes)
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

// resetEndpointStatus starts a fresh health snapshot for the current node
// set. In particular, RBACDenied must not remain latched after access is
// restored; recordEndpoint rebuilds it from the results of this sweep.
func (m *Monitor) resetEndpointStatus(nodes []corev1.Node) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, node := range nodes {
		m.endpoint[node.Name] = endpointStatus{}
	}
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
