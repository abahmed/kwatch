package kube

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Virtual entities for cluster services that are not Kubernetes objects.
var (
	APIServer  = knowledge.NewEntityID("apiserver", "", "kube-apiserver")
	ClusterDNS = knowledge.NewEntityID("cluster-dns", "", "cluster-dns")
)

// Probe results recorded on the virtual entities.
const (
	ProbeSource      = "probe"
	AttrHealthy      = "healthy"
	AttrLatencyMS    = "latency.ms"
	AttrProbeError   = "probe.error"
	probeInterval    = 30 * time.Second
	probeTimeout     = 5 * time.Second
	clusterDNSLookup = "kubernetes.default.svc"
)

// Resolver is the DNS lookup the probe uses.
type Resolver interface {
	LookupHost(context.Context, string) ([]string, error)
}

// ProbeConfig configures the control-plane and DNS prober.
type ProbeConfig struct {
	Client   kubernetes.Interface
	Resolver Resolver
	Now      func() time.Time
	Submit   Submit
}

// Prober checks the API server and cluster DNS every probeInterval. Each
// check has its own timeout, so a slow API server never makes DNS look
// broken.
type Prober struct {
	cfg ProbeConfig
}

// NewProber builds a prober.
func NewProber(cfg ProbeConfig) *Prober { return &Prober{cfg: cfg} }

// Run probes until ctx ends.
func (p *Prober) Run(ctx context.Context) {
	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()
	for {
		p.cfg.Submit(ctx, p.apiServer(ctx), p.dns(ctx))
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Prober) apiServer(ctx context.Context) knowledge.Fact {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	start := p.cfg.Now()
	_, err := p.cfg.Client.Discovery().RESTClient().Get().
		AbsPath("/readyz").DoRaw(probeCtx)
	return probeFact(APIServer, p.cfg.Now(), p.cfg.Now().Sub(start), err)
}

func (p *Prober) dns(ctx context.Context) knowledge.Fact {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	start := p.cfg.Now()
	_, err := p.cfg.Resolver.LookupHost(probeCtx, clusterDNSLookup)
	return probeFact(ClusterDNS, p.cfg.Now(), p.cfg.Now().Sub(start), err)
}

func probeFact(
	id knowledge.EntityID, at time.Time, latency time.Duration, err error,
) knowledge.Fact {
	attrs := map[string]knowledge.Value{
		AttrHealthy:   knowledge.Bool(err == nil),
		AttrLatencyMS: knowledge.Number(float64(latency.Milliseconds())),
	}
	if err != nil {
		attrs[AttrProbeError] = knowledge.Text(truncate(err.Error()))
	}
	return knowledge.Fact{
		Kind: knowledge.Observed, Source: ProbeSource, At: at, Entity: id,
		Attributes: attrs,
	}
}
