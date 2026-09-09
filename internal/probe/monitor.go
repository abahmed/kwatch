package probe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/correlation"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/observe"
)

type Monitor struct {
	cfg        config.ActiveProbeMonitor
	correlator *correlation.Engine
	client     *http.Client
	timeout    time.Duration
	kclient    kubernetes.Interface
	mu         sync.Mutex
	failures   map[string]int
	successes  map[string]int
	// failing marks the targets that have actually reported a failure. A
	// target that has always been healthy has nothing to recover from, so it
	// must not resolve on every tick.
	failing     map[string]bool
	graph       *kwcontext.ResourceGraph
	now         func() time.Time
	namespaces  []string
	watchAll    bool
	allowed     func(string) bool
	autoTargets map[string]autoProbeTarget
	// serviceLister reads the controller's Service informer cache. Auto
	// service probing used to LIST every Service in scope on every interval
	// (30s by default) while the controller already watched them.
	serviceLister corev1lister.ServiceLister
}

// SetServiceLister wires the controller's Service cache.
func (m *Monitor) SetServiceLister(lister corev1lister.ServiceLister) {
	m.mu.Lock()
	m.serviceLister = lister
	m.mu.Unlock()
}

func (m *Monitor) SetKubernetesClient(client kubernetes.Interface) { m.kclient = client }

// servicesFor reads Services from the informer cache, or nil when the cache
// is unavailable so the caller falls back to the API server.
func (m *Monitor) servicesFor(namespace string) []*corev1.Service {
	m.mu.Lock()
	lister := m.serviceLister
	m.mu.Unlock()
	if lister == nil {
		return nil
	}
	var (
		services []*corev1.Service
		err      error
	)
	if namespace == "" {
		services, err = lister.List(labels.Everything())
	} else {
		services, err = lister.Services(namespace).List(labels.Everything())
	}
	if err != nil {
		return nil
	}
	return services
}

func (m *Monitor) SetGraph(graph *kwcontext.ResourceGraph) {
	m.graph = graph
}

func (m *Monitor) SetNamespaceScope(namespaces []string, watchAll bool) {
	m.mu.Lock()
	m.namespaces = append([]string(nil), namespaces...)
	m.watchAll = watchAll
	m.mu.Unlock()
}

func (m *Monitor) SetNamespaceFilter(allowed func(string) bool) {
	m.mu.Lock()
	m.allowed = allowed
	m.mu.Unlock()
}

func New(cfg config.ActiveProbeMonitor, correlator *correlation.Engine) *Monitor {
	return NewWithClient(cfg, correlator)
}

// NewWithClient builds a monitor with the application's shared HTTP client.
// This keeps active probes aligned with proxy and TLS settings.
func NewWithClient(
	cfg config.ActiveProbeMonitor,
	correlator *correlation.Engine,
	clients ...*http.Client,
) *Monitor {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	if len(clients) > 0 && clients[0] != nil {
		shared := *clients[0]
		shared.Timeout = timeout
		client = &shared
	}
	return &Monitor{
		cfg: cfg, correlator: correlator,
		watchAll: true,
		client:   client,
		timeout:  timeout,
		failures: make(map[string]int), successes: make(map[string]int),
		failing:     make(map[string]bool),
		autoTargets: make(map[string]autoProbeTarget),
		now:         time.Now,
	}
}

// SetClock injects the clock used for active-probe latency measurements.
func (m *Monitor) SetClock(now func() time.Time) {
	if now != nil {
		m.now = now
	}
}

func (m *Monitor) nowTime() time.Time {
	if m.now != nil {
		return m.now()
	}
	return clock.Now()
}

func (m *Monitor) Start(ctx context.Context) {
	if !m.cfg.Enabled {
		return
	}
	interval := time.Duration(m.cfg.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	m.check(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx)
		}
	}
}

func (m *Monitor) check(ctx context.Context) {
	for _, target := range m.cfg.HTTP {
		m.linkTarget("http/"+target.Name, target.URL)
		ok, detail, reason := m.http(ctx, target)
		m.record("http/"+target.Name, "http/"+target.Name, reason, ok, detail)
	}
	for _, target := range m.cfg.TCP {
		m.linkTarget("tcp/"+target.Name, target.Address)
		ok, detail := m.tcp(ctx, target)
		m.record(
			"tcp/"+target.Name, "tcp/"+target.Name,
			constant.ReasonActiveProbeFailure, ok, detail,
		)
	}
	for _, target := range m.cfg.DNS {
		m.linkTarget("dns/"+target.Name, target.Host)
		ok, detail := m.dns(ctx, target)
		m.record(
			"dns/"+target.Name, "dns/"+target.Name,
			constant.ReasonActiveProbeFailure, ok, detail,
		)
	}
	if m.cfg.AutoServices && m.kclient != nil {
		m.checkServices(ctx)
	}
}

func (m *Monitor) linkTarget(owner, raw string) {
	if m.graph == nil {
		return
	}
	host := raw
	if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	} else if parsedHost, _, err := net.SplitHostPort(raw); err == nil {
		host = parsedHost
	}
	if svc, namespace, ok := serviceDNS(host); ok {
		m.graph.ReplaceOutgoingEdges("activeprobe", "", owner, []kwcontext.EdgeTarget{{Kind: "service", Namespace: namespace, Name: svc, Type: "probes"}})
		return
	}
	m.graph.ReplaceOutgoingEdges("activeprobe", "", owner, []kwcontext.EdgeTarget{{Kind: "networktarget", Name: owner, Type: "probes"}})
}

func serviceDNS(host string) (string, string, bool) {
	parts := strings.Split(strings.TrimSuffix(host, "."), ".")
	if len(parts) < 3 || parts[2] != "svc" || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (m *Monitor) http(ctx context.Context, target config.HTTPProbeTarget) (bool, string, string) {
	started := m.nowTime()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return false, err.Error(), constant.ReasonActiveProbeFailure
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return false, err.Error(), constant.ReasonActiveProbeFailure
	}
	_ = resp.Body.Close()
	latency := m.nowTime().Sub(started)
	expected := target.ExpectedStatus
	if expected == 0 {
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
			return m.probeLatencyResult(resp.StatusCode, latency, target)
		}
		return false, fmt.Sprintf("HTTP status %d (expected 2xx or 3xx)", resp.StatusCode), constant.ReasonActiveProbeFailure
	}
	if resp.StatusCode != expected {
		return false, fmt.Sprintf("HTTP status %d (expected %d)", resp.StatusCode, expected), constant.ReasonActiveProbeFailure
	}
	return m.probeLatencyResult(resp.StatusCode, latency, target)
}

func (m *Monitor) probeLatencyResult(status int, latency time.Duration, target config.HTTPProbeTarget) (bool, string, string) {
	return probeLatencyResult(status, latency, target)
}

func probeLatencyResult(status int, latency time.Duration, target config.HTTPProbeTarget) (bool, string, string) {
	detail := fmt.Sprintf("HTTP status %d in %s", status, latency.Round(time.Millisecond))
	if target.LatencyCriticalMs > 0 && latency >= time.Duration(target.LatencyCriticalMs)*time.Millisecond {
		return false, detail + fmt.Sprintf(" (critical latency threshold %dms exceeded)", target.LatencyCriticalMs), constant.ReasonActiveProbeLatency
	}
	if target.LatencyWarningMs > 0 && latency >= time.Duration(target.LatencyWarningMs)*time.Millisecond {
		return false, detail + fmt.Sprintf(" (latency threshold %dms exceeded)", target.LatencyWarningMs), constant.ReasonActiveProbeLatency
	}
	return true, detail, constant.ReasonActiveProbeFailure
}

func (m *Monitor) tcp(ctx context.Context, target config.TCPProbeTarget) (bool, string) {
	dialer := net.Dialer{Timeout: m.client.Timeout}
	probeCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	conn, err := dialer.DialContext(probeCtx, "tcp", target.Address)
	if err != nil {
		return false, err.Error()
	}
	_ = conn.Close()
	return true, "TCP connection established"
}

func (m *Monitor) dns(ctx context.Context, target config.DNSProbeTarget) (bool, string) {
	probeCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupHost(probeCtx, target.Host)
	if err != nil {
		return false, err.Error()
	}
	if len(addresses) == 0 {
		return false, "DNS returned no addresses"
	}
	return true, fmt.Sprintf("DNS resolved to %d address(es)", len(addresses))
}

func (m *Monitor) record(key, owner, reason string, ok bool, detail string) {
	m.mu.Lock()
	if ok {
		if !m.failing[key] {
			// This target was never reported failing, so there is nothing to
			// recover from. Resolving anyway cost a locked engine scan per
			// healthy target per tick, on a target that had never had an
			// incident in the first place.
			delete(m.failures, key)
			delete(m.successes, key)
			m.mu.Unlock()
			return
		}
		m.failures[key] = 0
		m.successes[key]++
		if m.successes[key] < threshold(m.cfg.RecoveryThreshold, 1) {
			m.mu.Unlock()
			return
		}
		delete(m.failures, key)
		delete(m.successes, key)
		delete(m.failing, key)
	} else {
		m.successes[key] = 0
		m.failures[key]++
		if m.failures[key] < threshold(m.cfg.FailureThreshold, 1) {
			m.mu.Unlock()
			return
		}
		m.failing[key] = true
	}
	m.mu.Unlock()

	if ok {
		m.correlator.Resolve(probeRef(owner), reason)
		if reason == constant.ReasonActiveProbeFailure {
			m.correlator.Resolve(
				probeRef(owner), constant.ReasonActiveProbeLatency,
			)
		}
		return
	}
	m.correlator.Process(
		observe.Synthetic("activeprobe", owner, reason).
			WithSeverity(model.SeverityWarning).
			WithHint(fmt.Sprintf("probe %s failed: %s", owner, detail)),
	)
}

// probeRef is the subject an active probe's incidents are about. A probe
// target is not a Kubernetes object, so its owner name is its whole identity.
func probeRef(owner string) model.ObjectRef {
	return model.ObjectRef{Kind: "activeprobe", Name: owner}
}

func threshold(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
