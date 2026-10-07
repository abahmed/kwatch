package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
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
	// HTTP reaches pods directly, for the cluster DNS metrics; nil
	// skips them.
	HTTP *http.Client
	// Model finds the cluster DNS pods; nil skips their metrics.
	Model inventory.Reader
	// OwnLease is kwatch's own leader Lease, which the Lease scan
	// leaves out: kwatch is the one renewing it. Empty names skip
	// nothing.
	OwnLeaseNamespace, OwnLeaseName string
}

// Prober checks the API server and cluster DNS every probeInterval. Each
// check has its own timeout, so a slow API server never makes DNS look
// broken. Every leaseScanEvery rounds it also reads the Leases of
// controllers and operators (see leases).
type Prober struct {
	cfg    ProbeConfig
	rounds int
	// rates turns the request and response counters into per-second
	// rates between rounds.
	rates *counterRates
	// leasesSeen are the Leases the last scan reported. Only the Run
	// goroutine touches it.
	leasesSeen map[inventory.EntityID]bool
	// deprecatedSeen is when each deprecated API was last reported by
	// the API server's metrics.
	deprecatedSeen map[inventory.EntityID]time.Time
	// plane remembers the control-plane metrics of each API server
	// process, to measure the calls made between two rounds.
	plane planeMemory
}

// LeaseScanPeriod is the time between two Lease scans. A detector that
// judges a Lease by its last renewal must wait longer than this before it
// calls a holder stuck, or it raises and clears on every scan.
const LeaseScanPeriod = leaseScanEvery * probeInterval

// Lease scan limits. Leases renew every few seconds, so they are read on
// a slow cadence and never watched; a bounded list keeps a cluster with
// thousands of leases from turning the scan into a load.
const (
	leaseScanEvery = 4
	leaseScanLimit = 500
	// leaseScanPages bounds one scan to leaseScanLimit*leaseScanPages
	// Leases.
	leaseScanPages = 20
	// nodeLeaseNamespace holds the kubelet heartbeats, read through the
	// node conditions instead.
	nodeLeaseNamespace  = "kube-node-lease"
	defaultLeaseSeconds = 15
)

// NewProber builds a prober.
func NewProber(cfg ProbeConfig) *Prober {
	return &Prober{cfg: cfg, rates: newCounterRates()}
}

// Run probes until ctx ends.
func (p *Prober) Run(ctx context.Context) {
	ticker := time.NewTicker(probeInterval)
	defer ticker.Stop()
	for {
		observations := append(p.apiServer(ctx), p.dns(ctx))
		observations = append(observations, p.leaders(ctx)...)
		if p.rounds%leaseScanEvery == 0 {
			observations = append(observations, p.leases(ctx)...)
		}
		p.rounds++
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
	observations := []inventory.Observation{api}
	if err == nil {
		p.addServerVersion(probeCtx, api.Attributes)
		observations = append(observations,
			p.addServerMetrics(probeCtx, api.Attributes, now)...)
	}
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
	out := probeObservation(ClusterDNS, p.cfg.Now(), p.cfg.Now().Sub(start),
		err)
	p.addDNSMetrics(probeCtx, out.Attributes, p.cfg.Now())
	return out
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
		// Always written on a failure: an unknown kind must replace
		// the previous failure's, not inherit it.
		attrs[AttrProbeFailureKind] = inventory.Text(probeFailureKind(err))
	}
	return inventory.Observation{
		Kind: inventory.Observed, Source: ProbeSource, At: at, Entity: id,
		Attributes: attrs,
	}
}

// leases records every Lease outside the node heartbeats and the
// control-plane leaders: who holds it, when it was last renewed and
// for how long it is valid. The detector decides what is stale; the
// prober only reports. An error (no RBAC, a managed control plane that
// hides them) reports nothing. Leases are only known through this scan,
// so a Lease the last complete scan saw and this one did not is gone. An
// incomplete scan (a failed page, too many pages) retires nothing.
func (p *Prober) leases(ctx context.Context) []inventory.Observation {
	items, complete := p.listLeases(ctx)
	now := p.cfg.Now()
	var out []inventory.Observation
	seen := make(map[inventory.EntityID]bool, len(items))
	for _, lease := range items {
		if p.skipLease(lease) {
			continue
		}
		seconds := float64(defaultLeaseSeconds)
		if lease.Spec.LeaseDurationSeconds != nil {
			seconds = float64(*lease.Spec.LeaseDurationSeconds)
		}
		id := inventory.CoreID(KindLease, lease.Namespace, lease.Name)
		seen[id] = true
		out = append(out, inventory.Observation{
			Kind: inventory.Observed, Source: ProbeSource, At: now,
			Entity: id,
			Attributes: map[string]inventory.Value{
				AttrLeaseHolder:   inventory.Text(*lease.Spec.HolderIdentity),
				AttrLeaseRenewed:  inventory.Time(lease.Spec.RenewTime.Time),
				AttrLeaseDuration: inventory.Number(seconds),
			},
		})
		if related, ok := p.holderRelation(id, *lease.Spec.HolderIdentity); ok {
			out = append(out, related)
		}
	}
	if !complete {
		for id := range p.leasesSeen {
			seen[id] = true
		}
	}
	for id := range p.leasesSeen {
		if !seen[id] {
			out = append(out, inventory.Observation{
				Kind: inventory.Gone, Source: ProbeSource, At: now,
				Entity: id,
			})
		}
	}
	p.leasesSeen = seen
	return out
}

// skipLease is a Lease the scan leaves out: node heartbeats, the
// control-plane leaders, kwatch's own and one nobody holds or renews.
func (p *Prober) skipLease(lease coordinationv1.Lease) bool {
	return lease.Namespace == nodeLeaseNamespace ||
		lease.Spec.RenewTime == nil || lease.Spec.HolderIdentity == nil ||
		p.isOwnLease(lease.Namespace, lease.Name) ||
		(lease.Namespace == "kube-system" &&
			(lease.Name == Scheduler.Name ||
				lease.Name == ControllerManager.Name))
}

// isOwnLease reports kwatch's own leader Lease.
func (p *Prober) isOwnLease(namespace, name string) bool {
	return p.cfg.OwnLeaseName != "" && name == p.cfg.OwnLeaseName &&
		namespace == p.cfg.OwnLeaseNamespace
}

// listLeases reads the Leases of every namespace page by page. complete
// is false when a page failed or the page cap was reached.
func (p *Prober) listLeases(
	ctx context.Context,
) (items []coordinationv1.Lease, complete bool) {
	token := ""
	for page := 0; page < leaseScanPages; page++ {
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		list, err := p.cfg.Client.CoordinationV1().Leases("").List(probeCtx,
			metav1.ListOptions{Limit: leaseScanLimit, Continue: token})
		cancel()
		if err != nil {
			return items, false
		}
		items = append(items, list.Items...)
		if token = list.Continue; token == "" {
			return items, true
		}
	}
	return items, false
}
