package kube

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Metrics the prober reads from the API server's own /metrics and from
// the cluster DNS pods' metrics port. Both are part of Kubernetes
// itself; a cluster without them, or without RBAC for /metrics, loses
// only these attributes.
const (
	// AttrAPIErrorRate is the API server's 5xx responses per second.
	AttrAPIErrorRate = "api.errors.per.second"
	// AttrAPIRequestRate is the API server's responses per second.
	AttrAPIRequestRate = "api.requests.per.second"
	// AttrDNSServfailRate is the cluster DNS's SERVFAIL answers per
	// second, summed over its pods.
	AttrDNSServfailRate = "dns.servfail.per.second"
	// AttrDNSRequestRate is the cluster DNS's answers per second.
	AttrDNSRequestRate = "dns.requests.per.second"
	// dnsMetricsPort is where CoreDNS and kube-dns expose metrics.
	dnsMetricsPort = "9153"
	// maxProbeMetricsBytes bounds one metrics response. The API server's
	// is large on a big cluster; the lines of interest are few.
	maxProbeMetricsBytes = 32 << 20
)

// DNSServerNames are the Deployments that serve the cluster DNS.
var DNSServerNames = map[string]bool{"coredns": true, "kube-dns": true}

// addServerMetrics reads the API server's request counters and records
// the overall and the 5xx response rates since the previous round.
func (p *Prober) addServerMetrics(
	ctx context.Context, attrs map[string]inventory.Value, now time.Time,
) {
	body, err := p.cfg.Client.Discovery().RESTClient().Get().
		AbsPath("/metrics").DoRaw(ctx)
	if err != nil {
		return
	}
	total, errors := requestCounts(body)
	if rate, ok := p.rates.rate("api/requests", now, total); ok {
		attrs[AttrAPIRequestRate] = inventory.Number(rate)
	}
	if rate, ok := p.rates.rate("api/errors", now, errors); ok {
		attrs[AttrAPIErrorRate] = inventory.Number(rate)
	}
}

// requestCounts sums apiserver_request_total, and the share of it whose
// code is a 5xx.
func requestCounts(body []byte) (total, errors float64) {
	forEachMetric(body, "apiserver_request_total", func(
		labels map[string]string, value float64,
	) {
		total += value
		if code := labels["code"]; len(code) == 3 && code[0] == '5' {
			errors += value
		}
	})
	return total, errors
}

// addDNSMetrics reads every cluster DNS pod's metrics and records the
// summed answer and SERVFAIL rates since the previous round.
func (p *Prober) addDNSMetrics(
	ctx context.Context, attrs map[string]inventory.Value, now time.Time,
) {
	if p.cfg.HTTP == nil || p.cfg.Model == nil {
		return
	}
	var total, servfail float64
	read := false
	for _, ip := range p.dnsPodIPs() {
		body, err := p.fetch(ctx, "http://"+net.JoinHostPort(ip,
			dnsMetricsPort)+"/metrics")
		if err != nil {
			continue
		}
		read = true
		forEachMetric(body, "coredns_dns_responses_total", func(
			labels map[string]string, value float64,
		) {
			total += value
			if labels["rcode"] == "SERVFAIL" {
				servfail += value
			}
		})
	}
	if !read {
		return
	}
	if rate, ok := p.rates.rate("dns/requests", now, total); ok {
		attrs[AttrDNSRequestRate] = inventory.Number(rate)
	}
	if rate, ok := p.rates.rate("dns/servfail", now, servfail); ok {
		attrs[AttrDNSServfailRate] = inventory.Number(rate)
	}
}

// dnsPodIPs lists the IPs of the pods that serve the cluster DNS.
func (p *Prober) dnsPodIPs() []string {
	var out []string
	for _, pod := range p.cfg.Model.Entities(KindPod) {
		if pod.Namespace != "kube-system" {
			continue
		}
		entity, ok := p.cfg.Model.Entity(pod)
		if !ok || !DNSServerNames[ownerName(p.cfg.Model, pod)] {
			continue
		}
		if ip, ok := entity.Attribute(AttrPodIP); ok &&
			ip.Value.AsText() != "" {
			out = append(out, ip.Value.AsText())
		}
	}
	return out
}

// ownerName is the name of the pod's top owner: the Deployment behind
// its ReplicaSet, or the pod itself.
func ownerName(model inventory.Reader, pod inventory.EntityID) string {
	id := pod
	for range 3 {
		owners := model.Related(id, inventory.OwnedBy, inventory.Outgoing)
		if len(owners) == 0 {
			break
		}
		id = owners[0]
	}
	return id.Name
}

// fetch reads one metrics page, bounded.
func (p *Prober) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.cfg.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, maxProbeMetricsBytes))
}
