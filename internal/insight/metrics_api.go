package insight

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/model"
)

type MetricsAPIEvidence struct {
	Observed       bool
	Registered     bool
	Available      bool
	Service        model.ObjectRef
	EndpointsSeen  bool
	ReadyEndpoints int
	Reason         string
}

func (e *Engine) inspectMetricsAPI(
	inc *model.Incident,
	ins *Insight,
) {
	if inc == nil || ins == nil || !isMetricsFailureReason(inc.Reason) ||
		e.metricsAPIInspector == nil {
		return
	}
	evidence := e.metricsAPIInspector()
	if !evidence.Observed {
		return
	}
	switch {
	case !evidence.Registered:
		ins.Cause = "the Kubernetes Metrics API is not registered"
		ins.Pattern = "metrics_api_failure"
		ins.Evidence = append(ins.Evidence,
			"APIService v1beta1.metrics.k8s.io is absent")
		ins.Confidence = 0.99
	case evidence.EndpointsSeen && evidence.ReadyEndpoints == 0 &&
		evidence.Service.Name != "":
		ins.Cause = fmt.Sprintf(
			"the Metrics API has no healthy endpoints behind service %s",
			evidence.Service.Describe(),
		)
		ins.Pattern = "metrics_api_failure"
		ins.Evidence = append(ins.Evidence,
			"the backing service has 0 ready endpoints")
		if evidence.Reason != "" {
			ins.Evidence = append(ins.Evidence,
				"the APIService reports "+evidence.Reason)
		}
		ins.Confidence = 0.99
	case !evidence.Available:
		ins.Cause = "the Kubernetes Metrics API is unavailable"
		ins.Pattern = "metrics_api_failure"
		ins.Evidence = append(ins.Evidence,
			"APIService v1beta1.metrics.k8s.io reports unavailable")
		if evidence.Reason != "" {
			ins.Evidence = append(ins.Evidence,
				"the APIService reports "+evidence.Reason)
		}
		ins.Confidence = 0.99
	case evidence.Available:
		ins.Contradictions = append(ins.Contradictions,
			"the Metrics APIService is currently available")
		if ins.Pattern == "metrics_api_failure" {
			ins.Cause = "the HPA could not obtain usable metrics for its target"
			ins.Pattern = "hpa_metrics_failure"
			ins.Confidence = 0.75
		}
	}
}
