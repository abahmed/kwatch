package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
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
	// restricted is whether the last round found every dependency
	// unreachable. Only the Run goroutine touches it.
	restricted bool
	// probed holds the external endpoints probed so far, with the rounds
	// since a pod last called them. Only the Run goroutine touches it.
	probed map[inventory.EntityID]int
	// capLogged is set once the Service cap has been reported.
	capLogged bool
	// dependencyDial opens connections for dependency probes: the same
	// as cfg.Dial when the caller set one, else a dialer that refuses
	// private addresses.
	dependencyDial func(context.Context, string, string) (net.Conn, error)
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
	dependencyDial := cfg.Dial
	if cfg.Dial == nil {
		cfg.Dial = (&net.Dialer{}).DialContext
		dependencyDial = externalDialer()
	}
	return &ActiveProber{cfg: cfg, dependencyDial: dependencyDial}
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
	p.stampLimits(&observation)
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

func (p *ActiveProber) dial(ctx context.Context, address string) error {
	return closeDialed(p.cfg.Dial(ctx, "tcp", address))
}

func closeDialed(conn net.Conn, err error) error {
	if err != nil {
		return err
	}
	return conn.Close()
}

// dependencyTargets lists the external endpoints the pods in scope are
// configured to call, each once, bounded like Service probing. It also
// returns every endpoint called, probed or not, so the ones the cap left
// out are not taken for abandoned.
func (p *ActiveProber) dependencyTargets() (
	[]inventory.EntityID, map[inventory.EntityID]bool,
) {
	called := map[inventory.EntityID]bool{}
	var out []inventory.EntityID
	for _, pod := range p.cfg.Model.Entities(KindPod) {
		if p.cfg.Excluded[pod.Namespace] {
			continue
		}
		for _, endpoint := range p.cfg.Model.Related(pod, inventory.Calls,
			inventory.Outgoing) {
			// In-cluster Services are called too (see
			// podServiceCalls); they are not dialled from here.
			if endpoint.Kind != KindExternalEndpoint || called[endpoint] {
				continue
			}
			called[endpoint] = true
			if len(out) < autoProbeLimit {
				out = append(out, endpoint)
			}
		}
	}
	return out, called
}

// checkDependency opens one TCP connection to the endpoint named
// "host:port" and records the result on the endpoint entity.
func (p *ActiveProber) checkDependency(
	ctx context.Context, endpoint inventory.EntityID,
) inventory.Observation {
	probeCtx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
	defer cancel()
	start := p.cfg.Now()
	err := closeDialed(p.dependencyDial(probeCtx, "tcp", endpoint.Name))
	observation := probeObservation(endpoint, p.cfg.Now(),
		p.cfg.Now().Sub(start), err)
	if errors.Is(err, errBlockedAddress) {
		// Not an outage of the dependency: kwatch refused to dial it.
		delete(observation.Attributes, AttrHealthy)
		delete(observation.Attributes, AttrProbeFailureKind)
		observation.Attributes[AttrProbeError] = inventory.Text(
			errBlockedAddress.Error())
	}
	observation.Source = dependencyProbeSource
	p.stampLimits(&observation)
	return observation
}
