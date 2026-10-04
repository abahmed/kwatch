package kube

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// KindEndpoint is the virtual entity for a user-configured probe target.
const KindEndpoint inventory.Kind = "endpoint"

// Attributes written by active probes.
const (
	activeProbeSource = "active-probe"
	// serviceProbeSource only enriches Services the informers observe, so
	// a probe that finishes after a Service is deleted cannot resurrect it.
	// Configured targets keep activeProbeSource: the prober owns those
	// virtual endpoint entities.
	serviceProbeSource = "active-probe-service"
	// dependencyProbeSource writes the external endpoints pods are
	// configured to call; the prober owns those entities too.
	dependencyProbeSource = "active-probe-dependency"
	AttrLatencyWarnMS     = "latency.warning.ms"
	AttrLatencyCritMS     = "latency.critical.ms"
	AttrFailureDuration   = "failure.threshold.seconds"
	autoProbeLimit        = 200
)

// ProbeTarget is one user-configured check. Exactly one of URL, Address
// or Host is set.
type ProbeTarget struct {
	Name              string
	URL               string
	ExpectedStatus    int
	LatencyWarningMs  int
	LatencyCriticalMs int
	Address           string
	Host              string
}

// ActiveProbeConfig configures user-defined probes.
type ActiveProbeConfig struct {
	Targets      []ProbeTarget
	AutoServices bool
	// AutoDependencies probes, over TCP, every external endpoint a pod
	// in scope is configured to call (see podDependencies).
	AutoDependencies bool
	Excluded         map[string]bool
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold int
	HTTPClient       *http.Client
	Resolver         Resolver
	// Dial opens TCP connections; nil uses a net.Dialer.
	Dial   func(ctx context.Context, network, address string) (net.Conn, error)
	Model  inventory.Reader
	Now    func() time.Time
	Submit Submit
}

// ActiveProber runs user-configured HTTP, TCP and DNS probes and, when
// enabled, a TCP probe of every Service port.
type ActiveProber struct {
	cfg ActiveProbeConfig
}

// NewActiveProber builds a prober with defaults for unset intervals.
func NewActiveProber(cfg ActiveProbeConfig) *ActiveProber {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.Dial == nil {
		cfg.Dial = (&net.Dialer{}).DialContext
	}
	return &ActiveProber{cfg: cfg}
}

// Run probes every interval until ctx ends.
func (p *ActiveProber) Run(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()
	for {
		p.round(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *ActiveProber) round(ctx context.Context) {
	var (
		mu           sync.Mutex
		observations []inventory.Observation
		wg           sync.WaitGroup
	)
	collect := func(observation inventory.Observation) {
		mu.Lock()
		observations = append(observations, observation)
		mu.Unlock()
	}
	for _, target := range p.cfg.Targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			collect(p.check(ctx, target))
		}()
	}
	if p.cfg.AutoServices {
		for _, target := range p.serviceTargets() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				collect(p.checkService(ctx, target))
			}()
		}
	}
	if p.cfg.AutoDependencies {
		for _, endpoint := range p.dependencyTargets() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				collect(p.checkDependency(ctx, endpoint))
			}()
		}
	}
	wg.Wait()
	if len(observations) > 0 {
		p.cfg.Submit(ctx, observations...)
	}
}

func (p *ActiveProber) check(
	ctx context.Context, target ProbeTarget,
) inventory.Observation {
	probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	start := p.cfg.Now()
	var err error
	var expiry time.Time
	switch {
	case target.URL != "":
		expiry, err = p.http(probeCtx, target)
	case target.Address != "":
		err = p.dial(probeCtx, target.Address)
	case target.Host != "":
		_, err = p.cfg.Resolver.LookupHost(probeCtx, target.Host)
	default:
		err = fmt.Errorf("probe %q has no target", target.Name)
	}
	observation := probeObservation(
		inventory.CoreID(KindEndpoint, "", target.Name),
		p.cfg.Now(), p.cfg.Now().Sub(start), err)
	observation.Source = activeProbeSource
	observation.Attributes[AttrFailureDuration] = inventory.Number(
		float64(p.cfg.FailureThreshold) * p.cfg.Interval.Seconds())
	if !expiry.IsZero() {
		observation.Attributes[AttrCertExpiry] = inventory.Time(expiry)
	}
	if target.LatencyWarningMs > 0 {
		observation.Attributes[AttrLatencyWarnMS] = inventory.Number(
			float64(target.LatencyWarningMs))
	}
	if target.LatencyCriticalMs > 0 {
		observation.Attributes[AttrLatencyCritMS] = inventory.Number(
			float64(target.LatencyCriticalMs))
	}
	return observation
}

// http fetches the target and returns the expiry of the certificate the
// server presented (zero over plain HTTP), so an expiring certificate is
// seen from the client side, where it matters.
func (p *ActiveProber) http(
	ctx context.Context, target ProbeTarget,
) (time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL,
		nil)
	if err != nil {
		return time.Time{}, err
	}
	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	var expiry time.Time
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		expiry = resp.TLS.PeerCertificates[0].NotAfter
	}
	expected := target.ExpectedStatus
	switch {
	case expected == 0 && resp.StatusCode >= 200 && resp.StatusCode < 400:
		return expiry, nil
	case expected != 0 && resp.StatusCode == expected:
		return expiry, nil
	default:
		return expiry, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
}

// serviceTarget is one Service port for automatic probing.
type serviceTarget struct {
	service inventory.EntityID
	address string
}

// serviceTargets lists ClusterIP Service ports from the model, bounded so
// a large cluster cannot turn probing into a scan.
func (p *ActiveProber) serviceTargets() []serviceTarget {
	var out []serviceTarget
	for _, id := range p.cfg.Model.Entities(KindService) {
		if p.cfg.Excluded[id.Namespace] || len(out) >= autoProbeLimit {
			continue
		}
		entity, ok := p.cfg.Model.Entity(id)
		if !ok {
			continue
		}
		attr, ok := entity.Attribute(AttrPorts)
		if !ok || attr.Value.AsText() == "" {
			continue
		}
		port := firstPort(attr.Value.AsText())
		if port == "" {
			continue
		}
		out = append(out, serviceTarget{service: id,
			address: net.JoinHostPort(id.Name+"."+id.Namespace+".svc", port)})
	}
	return out
}

func (p *ActiveProber) checkService(
	ctx context.Context, target serviceTarget,
) inventory.Observation {
	probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	start := p.cfg.Now()
	err := p.dial(probeCtx, target.address)
	observation := probeObservation(target.service, p.cfg.Now(),
		p.cfg.Now().Sub(start), err)
	observation.Source = serviceProbeSource
	observation.Attributes[AttrFailureDuration] = inventory.Number(
		float64(p.cfg.FailureThreshold) * p.cfg.Interval.Seconds())
	return observation
}

// firstPort reads the first port number from the "port/proto->target"
// list a ServiceSchema records.
func firstPort(ports string) string {
	for i, r := range ports {
		if r < '0' || r > '9' {
			if _, err := strconv.Atoi(ports[:i]); err == nil {
				return ports[:i]
			}
			return ""
		}
	}
	return ports
}

func (p *ActiveProber) dial(ctx context.Context, address string) error {
	conn, err := p.cfg.Dial(ctx, "tcp", address)
	if err != nil {
		return err
	}
	return conn.Close()
}

// dependencyTargets lists the external endpoints the pods in scope are
// configured to call, each once, bounded like Service probing.
func (p *ActiveProber) dependencyTargets() []inventory.EntityID {
	seen := map[inventory.EntityID]bool{}
	var out []inventory.EntityID
	for _, pod := range p.cfg.Model.Entities(KindPod) {
		if p.cfg.Excluded[pod.Namespace] {
			continue
		}
		for _, endpoint := range p.cfg.Model.Related(pod, inventory.Calls,
			inventory.Outgoing) {
			if seen[endpoint] || len(out) >= autoProbeLimit {
				continue
			}
			seen[endpoint] = true
			out = append(out, endpoint)
		}
	}
	return out
}

// checkDependency opens one TCP connection to the endpoint named
// "host:port" and records the result on the endpoint entity.
func (p *ActiveProber) checkDependency(
	ctx context.Context, endpoint inventory.EntityID,
) inventory.Observation {
	probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	start := p.cfg.Now()
	err := p.dial(probeCtx, endpoint.Name)
	observation := probeObservation(endpoint, p.cfg.Now(),
		p.cfg.Now().Sub(start), err)
	observation.Source = dependencyProbeSource
	observation.Attributes[AttrFailureDuration] = inventory.Number(
		float64(p.cfg.FailureThreshold) * p.cfg.Interval.Seconds())
	return observation
}
