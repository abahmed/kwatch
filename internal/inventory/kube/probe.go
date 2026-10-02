package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Virtual entities for cluster services that are not Kubernetes objects.
var (
	APIServer  = inventory.CoreID("apiserver", "", "kube-apiserver")
	ClusterDNS = inventory.CoreID("cluster-dns", "", "cluster-dns")
	Etcd       = inventory.CoreID("etcd", "", "etcd")
	Scheduler  = inventory.CoreID(
		"scheduler", "", "kube-scheduler")
	ControllerManager = inventory.CoreID(
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
		observations := append(p.apiServer(ctx), p.dns(ctx))
		observations = append(observations, p.leaders(ctx)...)
		p.cfg.Submit(ctx, observations...)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// apiServer checks /readyz?verbose, which also reports etcd.
func (p *Prober) apiServer(ctx context.Context) []inventory.Observation {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	start := p.cfg.Now()
	body, err := p.cfg.Client.Discovery().RESTClient().Get().
		AbsPath("/readyz").Param("verbose", "true").DoRaw(probeCtx)
	now := p.cfg.Now()
	api := probeObservation(APIServer, now, now.Sub(start), err)
	if err == nil {
		p.addServerVersion(probeCtx, api.Attributes)
	}
	observations := []inventory.Observation{api}
	if etcdErr, known := etcdCheck(string(body)); known {
		observations = append(observations, probeObservation(Etcd, now, 0, etcdErr))
	}
	return observations
}

// addServerVersion records the API server version for skew detection. The
// version is best effort: a failed read leaves the previous value in place.
func (p *Prober) addServerVersion(
	ctx context.Context, attrs map[string]inventory.Value,
) {
	raw, err := p.cfg.Client.Discovery().RESTClient().Get().
		AbsPath("/version").DoRaw(ctx)
	if err != nil {
		return
	}
	var info struct {
		GitVersion string `json:"gitVersion"`
	}
	if json.Unmarshal(raw, &info) != nil || info.GitVersion == "" {
		return
	}
	attrs[AttrServerVersion] = inventory.Text(info.GitVersion)
	if minor, ok := ParseMinor(info.GitVersion); ok {
		attrs[AttrServerMinor] = inventory.Number(float64(minor))
	}
}

// ParseMinor returns the minor number of a Kubernetes version such as
// "v1.29.3-gke.1". Distribution suffixes are ignored.
func ParseMinor(version string) (int, bool) {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 2 {
		return 0, false
	}
	digits := strings.TrimRightFunc(parts[1], func(r rune) bool {
		return r < '0' || r > '9'
	})
	minor, err := strconv.Atoi(digits)
	return minor, err == nil
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
func (p *Prober) leaders(ctx context.Context) []inventory.Observation {
	var observations []inventory.Observation
	for _, component := range []inventory.EntityID{
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
		observations = append(observations,
			probeObservation(component, p.cfg.Now(), 0, stale))
	}
	return observations
}

func (p *Prober) dns(ctx context.Context) inventory.Observation {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	start := p.cfg.Now()
	_, err := p.cfg.Resolver.LookupHost(probeCtx, clusterDNSLookup)
	return probeObservation(ClusterDNS, p.cfg.Now(), p.cfg.Now().Sub(start), err)
}

func probeObservation(
	id inventory.EntityID, at time.Time, latency time.Duration, err error,
) inventory.Observation {
	attrs := map[string]inventory.Value{
		AttrHealthy:   inventory.Bool(err == nil),
		AttrLatencyMS: inventory.Number(float64(latency.Milliseconds())),
	}
	if err != nil {
		attrs[AttrProbeError] = inventory.Text(evidenceText(err.Error()))
	}
	return inventory.Observation{
		Kind: inventory.Observed, Source: ProbeSource, At: at, Entity: id,
		Attributes: attrs,
	}
}
