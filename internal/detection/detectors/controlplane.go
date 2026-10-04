package detectors

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// DefaultProbeFailing is how long a probe must fail before it is a finding;
// one lost probe is noise.
const DefaultProbeFailing = 90 * time.Second

// ClusterService detects an unreachable API server and failing cluster
// DNS from kwatch's own probes.
type ClusterService struct{}

// Name implements detection.Detector.
func (ClusterService) Name() string { return "cluster-service" }

// Kinds implements detection.Detector.
func (ClusterService) Kinds() []inventory.Kind {
	return []inventory.Kind{
		kube.APIServer.Kind, kube.ClusterDNS.Kind, kube.Etcd.Kind,
		kube.Scheduler.Kind, kube.ControllerManager.Kind,
	}
}

// Detect implements detection.Detector.
func (ClusterService) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	healthy, known := e.Attribute(kube.AttrHealthy)
	if !known {
		return nil
	}
	if ok, _ := healthy.Value.AsBool(); ok {
		return append(apiLatency(ctx, e), errorRates(ctx, e)...)
	}
	if !sustained(ctx, "api-unhealthy", healthy.Since, DefaultProbeFailing) {
		return nil
	}
	reason, what := serviceReason(e.ID)
	return []detection.Finding{{
		Reason: reason, Severity: detection.Critical, Since: healthy.Since,
		Summary: what + " has been failing for " +
			format.Duration(ctx.Now.Sub(healthy.Since)),
		Evidence: errorEvidence(e),
	}}
}

func serviceReason(id inventory.EntityID) (string, string) {
	switch id {
	case kube.ClusterDNS:
		return reasons.CoreDNSUnavailable, "Cluster DNS"
	case kube.Etcd:
		return reasons.EtcdUnavailable, "etcd"
	case kube.Scheduler:
		return reasons.SchedulerUnavailable, "The scheduler"
	case kube.ControllerManager:
		return reasons.ControllerManagerUnavailable,
			"The controller manager"
	default:
		return reasons.APIServerUnavailable, "Kubernetes API"
	}
}

// slowAPI is the API response time above which controllers and kubectl
// visibly lag.
const slowAPI = 2000.0

func apiLatency(ctx detection.Context, e inventory.Entity) []detection.Finding {
	if e.ID != kube.APIServer {
		return nil
	}
	latency, ok := number(e, kube.AttrLatencyMS)
	if !ok || latency < slowAPI {
		return nil
	}
	// Latency changes by a few milliseconds on every probe; the wait
	// runs from when it first went over the threshold.
	since := ctx.Onset("slow", valueSince(e, kube.AttrLatencyMS))
	if !sustained(ctx, "slow", since, DefaultProbeFailing) {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.APIServerLatency, Severity: detection.Warning,
		Since:   since,
		Summary: "Kubernetes API is slow to respond",
	}}
}

// Error-rate thresholds for the API server and the cluster DNS, read
// from their own metrics by the prober. A share needs a floor of
// traffic: one failing request out of three is not a rate.
const (
	apiErrorShare    = 5.0
	dnsServfailShare = 10.0
	minRequestRate   = 1.0
)

// errorRates reports an API server answering many requests with 5xx,
// or a cluster DNS failing many lookups, while both still answer the
// probe: degraded, not down.
func errorRates(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	switch e.ID {
	case kube.APIServer:
		return shareFinding(ctx, e, "api-errors", kube.AttrAPIErrorRate,
			kube.AttrAPIRequestRate, apiErrorShare, reasons.APIServerErrors,
			"Kubernetes API answers %s of requests with server errors")
	case kube.ClusterDNS:
		return shareFinding(ctx, e, "dns-servfail", kube.AttrDNSServfailRate,
			kube.AttrDNSRequestRate, dnsServfailShare, reasons.CoreDNSServfail,
			"Cluster DNS fails %s of lookups with SERVFAIL")
	}
	return nil
}

// shareFinding reports failures as a share of requests above threshold,
// sustained for DefaultProbeFailing. summary takes the share.
func shareFinding(
	ctx detection.Context, e inventory.Entity, key, failedAttr,
	totalAttr string, threshold float64, reason, summary string,
) []detection.Finding {
	failed, ok1 := number(e, failedAttr)
	total, ok2 := number(e, totalAttr)
	if !ok1 || !ok2 || total < minRequestRate {
		return nil
	}
	share := 100 * failed / total
	if share < threshold {
		return nil
	}
	since := ctx.Onset(key, valueSince(e, failedAttr))
	if !sustained(ctx, key, since, DefaultProbeFailing) {
		return nil
	}
	return []detection.Finding{{
		Reason: reason, Severity: detection.Warning, Since: since,
		Summary: fmt.Sprintf(summary, percentText(share)),
		Evidence: []detection.Evidence{
			{Label: "failures per second", Value: fmt.Sprintf("%.1f", failed)},
			{Label: "requests per second", Value: fmt.Sprintf("%.1f", total)},
		},
	}}
}
