package kube

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Virtual entities for cluster services that are not Kubernetes objects.
var (
	APIServer  = knowledge.NewEntityID("apiserver", "", "kube-apiserver")
	ClusterDNS = knowledge.NewEntityID("cluster-dns", "", "cluster-dns")
	Etcd       = knowledge.NewEntityID("etcd", "", "etcd")
	Scheduler  = knowledge.NewEntityID(
		"scheduler", "", "kube-scheduler")
	ControllerManager = knowledge.NewEntityID(
		"controller-manager", "", "kube-controller-manager")
)

// leaseStale is how old a control-plane leader lease renewal may be.
const leaseStale = 90 * time.Second

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
		facts := append(p.apiServer(ctx), p.dns(ctx))
		facts = append(facts, p.leaders(ctx)...)
		p.cfg.Submit(ctx, facts...)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// apiServer checks /readyz?verbose, which also reports etcd.
func (p *Prober) apiServer(ctx context.Context) []knowledge.Fact {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	start := p.cfg.Now()
	body, err := p.cfg.Client.Discovery().RESTClient().Get().
		AbsPath("/readyz").Param("verbose", "true").DoRaw(probeCtx)
	now := p.cfg.Now()
	facts := []knowledge.Fact{
		probeFact(APIServer, now, now.Sub(start), err),
	}
	if etcdErr, known := etcdCheck(string(body)); known {
		facts = append(facts, probeFact(Etcd, now, 0, etcdErr))
	}
	return facts
}

// etcdCheck reads the etcd line of a verbose readyz report.
func etcdCheck(report string) (error, bool) {
	for _, line := range strings.Split(report, "\n") {
		switch {
		case strings.HasPrefix(line, "[+]etcd ok"):
			return nil, true
		case strings.HasPrefix(line, "[-]etcd"):
			return errors.New(strings.TrimSpace(line)), true
		}
	}
	return nil, false
}

// leaders checks the kube-system leader leases of the scheduler and the
// controller-manager. A lease that stopped renewing means the component
// is down, even on managed clusters where its pods are hidden.
func (p *Prober) leaders(ctx context.Context) []knowledge.Fact {
	var facts []knowledge.Fact
	for _, component := range []knowledge.EntityID{
		Scheduler, ControllerManager,
	} {
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		lease, err := p.cfg.Client.CoordinationV1().Leases("kube-system").
			Get(probeCtx, component.Name, metav1.GetOptions{})
		cancel()
		if err != nil || lease.Spec.RenewTime == nil {
			// Not visible here (managed control plane or no RBAC).
			continue
		}
		age := p.cfg.Now().Sub(lease.Spec.RenewTime.Time)
		var stale error
		if age > leaseStale {
			stale = fmt.Errorf("leader lease not renewed for %s",
				age.Round(time.Second))
		}
		facts = append(facts, probeFact(component, p.cfg.Now(), 0, stale))
	}
	return facts
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
