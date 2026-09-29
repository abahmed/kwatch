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
	return []knowledge.Kind{kube.APIServer.Kind, kube.ClusterDNS.Kind}
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
		return nil
	}
	if !sustained(ctx, healthy.Since, DefaultProbeFailing) {
		return nil
	}
	reason, what := constant.ReasonAPIServerUnavailable, "Kubernetes API"
	if e.ID == kube.ClusterDNS {
		reason, what = constant.ReasonCoreDNSUnavailable, "Cluster DNS"
	}
	return []signal.Signal{{
		Reason: reason, Severity: signal.Critical, Since: healthy.Since,
		Summary: what + " has been failing for " +
			format.Duration(ctx.Now.Sub(healthy.Since)),
		Evidence: []signal.Evidence{{
			Label: "error", Value: text(e, kube.AttrProbeError),
		}},
	}}
}
