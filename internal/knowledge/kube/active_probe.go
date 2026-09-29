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

	"github.com/abahmed/kwatch/internal/knowledge"
)

// KindEndpoint is the virtual entity for a user-configured probe target.
const KindEndpoint knowledge.Kind = "endpoint"

// Attributes written by active probes.
const (
	activeProbeSource   = "active-probe"
	AttrLatencyWarnMS   = "latency.warning.ms"
	AttrLatencyCritMS   = "latency.critical.ms"
	AttrFailureDuration = "failure.threshold.seconds"
	autoProbeLimit      = 200
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
	Targets          []ProbeTarget
	AutoServices     bool
	Excluded         map[string]bool
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold int
	HTTPClient       *http.Client
	Resolver         Resolver
	// Dial opens TCP connections; nil uses a net.Dialer.
	Dial   func(ctx context.Context, network, address string) (net.Conn, error)
	Model  knowledge.Reader
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
		mu    sync.Mutex
		facts []knowledge.Fact
		wg    sync.WaitGroup
	)
	collect := func(fact knowledge.Fact) {
		mu.Lock()
		facts = append(facts, fact)
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
	wg.Wait()
	if len(facts) > 0 {
		p.cfg.Submit(ctx, facts...)
	}
}

func (p *ActiveProber) check(
	ctx context.Context, target ProbeTarget,
) knowledge.Fact {
	probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	start := p.cfg.Now()
	var err error
	switch {
	case target.URL != "":
		err = p.http(probeCtx, target)
	case target.Address != "":
		err = p.dial(probeCtx, target.Address)
	case target.Host != "":
		_, err = p.cfg.Resolver.LookupHost(probeCtx, target.Host)
	default:
		err = fmt.Errorf("probe %q has no target", target.Name)
	}
	fact := probeFact(knowledge.NewEntityID(KindEndpoint, "", target.Name),
		p.cfg.Now(), p.cfg.Now().Sub(start), err)
	fact.Source = activeProbeSource
	fact.Attributes[AttrFailureDuration] = knowledge.Number(
		float64(p.cfg.FailureThreshold) * p.cfg.Interval.Seconds())
	if target.LatencyWarningMs > 0 {
		fact.Attributes[AttrLatencyWarnMS] = knowledge.Number(
			float64(target.LatencyWarningMs))
	}
	if target.LatencyCriticalMs > 0 {
		fact.Attributes[AttrLatencyCritMS] = knowledge.Number(
			float64(target.LatencyCriticalMs))
	}
	return fact
}

func (p *ActiveProber) http(ctx context.Context, target ProbeTarget) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL,
		nil)
	if err != nil {
		return err
	}
	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
	expected := target.ExpectedStatus
	switch {
	case expected == 0 && resp.StatusCode >= 200 && resp.StatusCode < 400:
		return nil
	case expected != 0 && resp.StatusCode == expected:
		return nil
	default:
		return fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
}

// serviceTarget is one Service port for automatic probing.
type serviceTarget struct {
	service knowledge.EntityID
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
) knowledge.Fact {
	probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	start := p.cfg.Now()
	err := p.dial(probeCtx, target.address)
	fact := probeFact(target.service, p.cfg.Now(), p.cfg.Now().Sub(start),
		err)
	fact.Source = activeProbeSource
	fact.Attributes[AttrFailureDuration] = knowledge.Number(
		float64(p.cfg.FailureThreshold) * p.cfg.Interval.Seconds())
	return fact
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
