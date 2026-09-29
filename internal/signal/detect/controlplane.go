package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// DefaultProbeFailing is how long a probe must fail before it is a signal;
// one lost probe is noise.
const DefaultProbeFailing = 90 * time.Second

// ClusterService detects an unreachable API server and failing cluster
// DNS from kwatch's own probes.
type ClusterService struct{}

// Name implements signal.Detector.
func (ClusterService) Name() string { return "cluster-service" }

// Kinds implements signal.Detector.
func (ClusterService) Kinds() []knowledge.Kind {
	return []knowledge.Kind{
		kube.APIServer.Kind, kube.ClusterDNS.Kind, kube.Etcd.Kind,
		kube.Scheduler.Kind, kube.ControllerManager.Kind,
	}
}

// Detect implements signal.Detector.
func (ClusterService) Detect(
	ctx signal.Context, e knowledge.Entity,
) []signal.Signal {
	healthy, known := e.Attribute(kube.AttrHealthy)
	if !known {
		return nil
	}
	if ok, _ := healthy.Value.AsBool(); ok {
		return apiLatency(ctx, e)
	}
	if !sustained(ctx, healthy.Since, DefaultProbeFailing) {
		return nil
	}
	reason, what := serviceReason(e.ID)
	return []signal.Signal{{
		Reason: reason, Severity: signal.Critical, Since: healthy.Since,
		Summary: what + " has been failing for " +
			format.Duration(ctx.Now.Sub(healthy.Since)),
		Evidence: []signal.Evidence{{
			Label: "error", Value: text(e, kube.AttrProbeError),
		}},
	}}
}

func serviceReason(id knowledge.EntityID) (string, string) {
	switch id {
	case kube.ClusterDNS:
		return constant.ReasonCoreDNSUnavailable, "Cluster DNS"
	case kube.Etcd:
		return constant.ReasonEtcdUnavailable, "etcd"
	case kube.Scheduler:
		return constant.ReasonSchedulerUnavailable, "The scheduler"
	case kube.ControllerManager:
		return constant.ReasonControllerManagerUnavailable,
			"The controller manager"
	default:
		return constant.ReasonAPIServerUnavailable, "Kubernetes API"
	}
}

// slowAPI is the API response time above which controllers and kubectl
// visibly lag.
const slowAPI = 2000.0

func apiLatency(ctx signal.Context, e knowledge.Entity) []signal.Signal {
	if e.ID != kube.APIServer {
		return nil
	}
	latency, ok := number(e, kube.AttrLatencyMS)
	if !ok || latency < slowAPI {
		return nil
	}
	since := valueSince(e, kube.AttrLatencyMS)
	if !sustained(ctx, since, DefaultProbeFailing) {
		return nil
	}
	return []signal.Signal{{
		Reason: constant.ReasonAPIServerLatency, Severity: signal.Warning,
		Since:   since,
		Summary: "Kubernetes API is slow to respond",
	}}
}
