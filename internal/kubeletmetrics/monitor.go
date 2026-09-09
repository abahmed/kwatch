package kubeletmetrics

import (
	"context"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/observe"
)

type Monitor struct {
	client     kubernetes.Interface
	correlator *correlation.Engine
	cfg        config.KubeletTelemetryMonitor
	previous   map[string]metricSnapshot
	failures   map[string]int
	successes  map[string]int
	// failing marks the signals that have actually crossed the failure
	// threshold, so a signal that has always been healthy never resolves.
	failing    map[string]bool
	stateSeen  map[string]time.Time
	baselines  map[string]usageBaseline
	now        func() time.Time
	mu         sync.Mutex
	endpoint   map[string]endpointStatus
	podCache   map[string]*corev1.Pod
	podCacheAt time.Time
	lastSweep  time.Time
	store      StateStore
	namespaces []string
	watchAll   bool
	allowed    func(string) bool
	// owners resolves a pod to the workload its incidents are keyed by; nil
	// falls back to keying by pod.
	owners observe.OwnerResolver
	// nodeLister reads the controller's node informer cache.
	nodeLister corev1lister.NodeLister
	// podLister reads the controller's pod informer cache.
	//
	// This monitor used to LIST every pod from the API server on every sweep
	// -- once a minute, paged at 500 -- while the controller already held a
	// synced pod informer for the same objects. One cluster-wide LIST per
	// minute per monitor is load the API server does not need to carry, and
	// two views of the same pods can disagree.
	podLister corev1lister.PodLister
}

// SetNodeLister wires the controller's node cache. When set, the sweep reads
// it instead of paging every node from the API server on each interval.
func (m *Monitor) SetNodeLister(lister corev1lister.NodeLister) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodeLister = lister
}

// SetPodLister wires the controller's pod cache. When set, sweeps read it
// instead of listing from the API server.
func (m *Monitor) SetPodLister(lister corev1lister.PodLister) {
	m.mu.Lock()
	m.podLister = lister
	m.mu.Unlock()
}

type StateStore interface {
	LoadTelemetryState(context.Context) ([]byte, error)
	SaveTelemetryState(context.Context, []byte) error
}

func New(
	client kubernetes.Interface,
	cfg config.KubeletTelemetryMonitor,
	correlator *correlation.Engine,
) *Monitor {
	return &Monitor{
		client: client, cfg: cfg, correlator: correlator, watchAll: true,
		previous: make(map[string]metricSnapshot),
		failures: make(map[string]int), successes: make(map[string]int),
		failing:   make(map[string]bool),
		stateSeen: make(map[string]time.Time),
		baselines: make(map[string]usageBaseline),
		endpoint:  make(map[string]endpointStatus),
		podCache:  make(map[string]*corev1.Pod), now: time.Now,
	}
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

func (m *Monitor) SetClock(now func() time.Time) {
	if now != nil {
		m.now = now
	}
}

func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.mu.Lock()
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
	m.mu.Unlock()
}

// SetOwnerResolver wires the lister-backed owner lookup the pod pipeline uses,
// so kubelet-derived incidents land on the same workload key as everything
// else. An unresolved answer falls back to the pod.
func (m *Monitor) SetOwnerResolver(owners observe.OwnerResolver) {
	m.mu.Lock()
	m.owners = owners
	m.mu.Unlock()
}

func (m *Monitor) SetNamespaceFilter(allowed func(string) bool) {
	m.mu.Lock()
	m.allowed = allowed
	m.mu.Unlock()
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
	lister := m.podLister
	m.mu.Unlock()
	if lister != nil {
		if result, ok := m.podsFromCache(lister); ok {
			m.mu.Lock()
			m.podCache, m.podCacheAt = result, now
			m.mu.Unlock()
			return result
		}
	}
	m.mu.Lock()
	namespaces := append([]string(nil), m.namespaces...)
	watchAll := m.watchAll
	allowed := m.allowed
	m.mu.Unlock()
	if !watchAll && len(namespaces) == 0 {
		return map[string]*corev1.Pod{}
	}
	result := make(map[string]*corev1.Pod)
	if watchAll {
		namespaces = []string{""}
	}
	for _, namespace := range namespaces {
		continueToken := ""
		for {
			pods, err := m.client.CoreV1().Pods(namespace).List(
				ctx, metav1.ListOptions{Limit: 500, Continue: continueToken},
			)
			if err != nil {
				return nil
			}
			for i := range pods.Items {
				pod := &pods.Items[i]
				if allowed != nil && !allowed(pod.Namespace) {
					continue
				}
				result[pod.Namespace+"/"+pod.Name] = pod
			}
			continueToken = pods.Continue
			if continueToken == "" {
				break
			}
		}
	}
	m.mu.Lock()
	m.podCache, m.podCacheAt = result, now
	m.mu.Unlock()
	return result
}

func (m *Monitor) nodes(ctx context.Context) ([]corev1.Node, error) {
	m.mu.Lock()
	lister := m.nodeLister
	m.mu.Unlock()
	if lister != nil {
		nodes, err := lister.List(labels.Everything())
		if err == nil {
			result := make([]corev1.Node, 0, len(nodes))
			for _, node := range nodes {
				result = append(result, *node)
			}
			return result, nil
		}
	}
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

// podsFromCache reads the pod informer cache, honouring the configured
// namespace scope. ok is false when the cache cannot answer, so the caller
// falls back to the API server rather than acting on an empty view.
func (m *Monitor) podsFromCache(
	lister corev1lister.PodLister,
) (map[string]*corev1.Pod, bool) {
	m.mu.Lock()
	namespaces := append([]string(nil), m.namespaces...)
	watchAll := m.watchAll
	allowed := m.allowed
	m.mu.Unlock()
	if !watchAll && len(namespaces) == 0 {
		return map[string]*corev1.Pod{}, true
	}
	if watchAll {
		namespaces = []string{""}
	}
	result := make(map[string]*corev1.Pod)
	for _, namespace := range namespaces {
		var (
			pods []*corev1.Pod
			err  error
		)
		if namespace == "" {
			pods, err = lister.List(labels.Everything())
		} else {
			pods, err = lister.Pods(namespace).List(labels.Everything())
		}
		if err != nil {
			return nil, false
		}
		for _, pod := range pods {
			if allowed != nil && !allowed(pod.Namespace) {
				continue
			}
			result[pod.Namespace+"/"+pod.Name] = pod
		}
	}
	return result, true
}
