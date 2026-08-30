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

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

type Monitor struct {
	cfg        config.ActiveProbeMonitor
	correlator *correlation.Engine
	client     *http.Client
	timeout    time.Duration
	mu         sync.Mutex
	failures   map[string]int
	successes  map[string]int
	graph      *kwcontext.ResourceGraph
}

func (m *Monitor) SetGraph(graph *kwcontext.ResourceGraph) {
	m.graph = graph
}

func New(cfg config.ActiveProbeMonitor, correlator *correlation.Engine) *Monitor {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Monitor{
		cfg: cfg, correlator: correlator,
		client:   &http.Client{Timeout: timeout},
		timeout:  timeout,
		failures: make(map[string]int), successes: make(map[string]int),
	}
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
		ok, detail := m.http(ctx, target)
		m.record("http/"+target.Name, "http/"+target.Name, ok, detail)
	}
	for _, target := range m.cfg.TCP {
		m.linkTarget("tcp/"+target.Name, target.Address)
		ok, detail := m.tcp(ctx, target)
		m.record("tcp/"+target.Name, "tcp/"+target.Name, ok, detail)
	}
	for _, target := range m.cfg.DNS {
		m.linkTarget("dns/"+target.Name, target.Host)
		ok, detail := m.dns(ctx, target)
		m.record("dns/"+target.Name, "dns/"+target.Name, ok, detail)
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

func (m *Monitor) http(ctx context.Context, target config.HTTPProbeTarget) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return false, err.Error()
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	_ = resp.Body.Close()
	expected := target.ExpectedStatus
	if expected == 0 {
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusBadRequest {
			return true, fmt.Sprintf("HTTP status %d", resp.StatusCode)
		}
		return false, fmt.Sprintf("HTTP status %d (expected 2xx or 3xx)", resp.StatusCode)
	}
	if resp.StatusCode != expected {
		return false, fmt.Sprintf("HTTP status %d (expected %d)", resp.StatusCode, expected)
	}
	return true, fmt.Sprintf("HTTP status %d", resp.StatusCode)
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

func (m *Monitor) record(key, owner string, ok bool, detail string) {
	m.mu.Lock()
	if ok {
		m.failures[key] = 0
		m.successes[key]++
		if m.successes[key] < threshold(m.cfg.RecoveryThreshold, 1) {
			m.mu.Unlock()
			return
		}
		delete(m.failures, key)
		delete(m.successes, key)
	} else {
		m.successes[key] = 0
		m.failures[key]++
		if m.failures[key] < threshold(m.cfg.FailureThreshold, 1) {
			m.mu.Unlock()
			return
		}
	}
	m.mu.Unlock()

	if ok {
		m.correlator.MarkResolved(correlation.BuildKey("", owner, constant.ReasonActiveProbeFailure, ""))
		return
	}
	m.correlator.Process(event.Event{
		Resource: "activeprobe", PodName: owner, Reason: constant.ReasonActiveProbeFailure,
		Hint: fmt.Sprintf("probe %s failed: %s", owner, detail), Severity: model.SeverityWarning,
	}, owner, nil)
}

func threshold(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}
