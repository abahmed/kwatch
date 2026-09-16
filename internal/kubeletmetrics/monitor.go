package kubeletmetrics

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/monitor"
	"github.com/abahmed/kwatch/internal/observe"
)

type Monitor struct {
	client       kubernetes.Interface
	incidentSink monitor.ObservationSink
	cfg          config.KubeletTelemetryMonitor
	previous     map[string]metricSnapshot
	failures     map[string]int
	successes    map[string]int
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
	podLister  corev1lister.PodLister
	configured bool
	started    bool
}

type StateStore interface {
	LoadTelemetryState(context.Context) ([]byte, error)
	SaveTelemetryState(context.Context, []byte) error
}

// NewWithClock constructs the kubelet monitor with an explicit clock.
func NewWithClock(
	client kubernetes.Interface,
	cfg config.KubeletTelemetryMonitor,
	incidentSink monitor.ObservationSink,
	timeSource clock.Clock,
) *Monitor {
	timeSource = clock.Require(timeSource)
	return &Monitor{
		client: client, cfg: cfg, incidentSink: incidentSink, watchAll: true,
		previous: make(map[string]metricSnapshot),
		failures: make(map[string]int), successes: make(map[string]int),
		failing:   make(map[string]bool),
		stateSeen: make(map[string]time.Time),
		baselines: make(map[string]usageBaseline),
		endpoint:  make(map[string]endpointStatus),
		podCache:  make(map[string]*corev1.Pod), now: timeSource.Now,
	}
}

func (m *Monitor) Start(ctx context.Context) error {
	if !m.cfg.Enabled || m.client == nil {
		return nil
	}
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	m.started = true
	m.mu.Unlock()
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
			return nil
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

func (m *Monitor) TelemetryStatus() Status {
	return m.Snapshot()
}

// StatusJSON implements the health status boundary.
func (m *Monitor) StatusJSON() ([]byte, error) {
	return json.Marshal(m.Snapshot())
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
