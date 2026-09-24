package insight

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

// scoreInsight combines detector output, graph evidence, and bounded feedback
// into the confidence shown with an insight. It is deliberately separate from
// Analyze so orchestration remains a short, readable sequence.
func (e *Engine) scoreInsight(inc *model.Incident, ins *Insight) {
	if inc == nil {
		return
	}
	evidenceBefore := e.appendObservedEvidence(inc, ins)
	e.setPatternConfidence(ins)
	e.applyInsightAdjustments(inc, ins, evidenceBefore)
	ins.NextSteps = nextSteps(inc)
}

func (e *Engine) appendObservedEvidence(
	inc *model.Incident,
	ins *Insight,
) int {
	if isMetricsFailureReason(inc.Reason) {
		ins.Evidence = append(
			ins.Evidence,
			"the HPA reported that required metrics could not be obtained",
		)
	}
	if inc.OwnerUnhealthy {
		ins.Evidence = append(ins.Evidence, "the owning workload is unhealthy")
	}
	if inc.Facts.MemoryLeak {
		ins.Evidence = append(ins.Evidence, fmt.Sprintf(
			"repeated OOM kills were observed in a %d-minute window",
			inc.Facts.OOMWindowMin,
		))
	}
	if inc.Facts.ProbeEndpoint != "" {
		ins.Evidence = append(
			ins.Evidence,
			"probe failed: "+inc.Facts.ProbeEndpoint,
		)
	}
	if inc.Facts.SchedulingDelay > 0 {
		ins.Evidence = append(ins.Evidence, fmt.Sprintf(
			"the workload has remained unscheduled for %s",
			inc.Facts.SchedulingDelay.Round(time.Second),
		))
	}
	if inc.Facts.PullSecretsSet {
		ins.Evidence = append(
			ins.Evidence,
			"the pod declares image pull secrets",
		)
	}
	if inc.Facts.Volume != "" {
		ins.Evidence = append(
			ins.Evidence,
			"bound volume: "+inc.Facts.Volume,
		)
	}
	if inc.Facts.MetricFailure != "" {
		detail := "Kubernetes classified the HPA failure as " +
			strings.ReplaceAll(inc.Facts.MetricFailure, "_", " ")
		if inc.Facts.MetricName != "" {
			detail += " for " + inc.Facts.MetricName
		}
		ins.Evidence = append(ins.Evidence, detail)
	}
	if inc.Facts.FailureDomain != "" {
		ins.Evidence = append(
			ins.Evidence,
			"Kubernetes reported a "+inc.Facts.FailureDomain+" failure",
		)
	}
	if len(ins.RecentChanges) > 0 {
		ins.Evidence = append(
			ins.Evidence,
			"a related resource changed shortly before the incident",
		)
	}
	evidenceBefore := len(ins.Evidence)
	e.appendActiveDependencyEvidence(inc, ins)
	return evidenceBefore
}

func (e *Engine) setPatternConfidence(ins *Insight) {
	switch ins.Pattern {
	case "node_failure":
		ins.Confidence = 0.90
	case "rollout_failure", "storage_failure", "storage_attachment_failure":
		ins.Confidence = 0.85
	case "dependency_change", "config_error":
		ins.Confidence = 0.65
	case "resource_limit":
		ins.Confidence = 0.85
	case "node_pressure":
		ins.Confidence = 0.80
	case "metrics_unavailable":
		ins.Confidence = 0.90
	case "metrics_api_failure", "hpa_missing_request", "hpa_invalid_metric":
		ins.Confidence = 0.95
	case "hpa_missing_pod_metrics":
		ins.Confidence = 0.90
	case "scheduling_failure", "admission_failure",
		"network_failure", "api_discovery_failure", "workload_failure",
		"scaling_failure", "configuration_failure", "health_check_failure":
		ins.Confidence = 0.95
	case "root_cause":
		ins.Confidence = 0.40
	}
}

func isMetricsFailureReason(reason string) bool {
	switch reason {
	case constant.ReasonFailedGetResourceMetric,
		constant.ReasonFailedComputeMetricsReplicas,
		constant.ReasonFailedGetMetrics:
		return true
	default:
		return false
	}
}

func (e *Engine) applyInsightAdjustments(
	inc *model.Incident,
	ins *Insight,
	evidenceBefore int,
) {
	e.applyFeedbackBias(inc, ins)
	if ins.Confidence > 0 && len(ins.Evidence) == 0 {
		ins.Confidence *= 0.65
	}
	if ins.Confidence > 0 && evidenceBefore == 0 && len(ins.Evidence) > 0 {
		ins.Confidence = minFloat(ins.Confidence+0.10, 1)
	}
}

func (e *Engine) appendActiveDependencyEvidence(
	inc *model.Incident,
	ins *Insight,
) {
	if e.activeChecker == nil || e.graph == nil {
		return
	}
	for _, dependency := range dependenciesFor(e.graph, inc) {
		ref, ok := model.ParseObjectKey(dependency)
		if !ok || !e.activeChecker(ref.Kind, ref.Namespace, ref.Name) {
			continue
		}
		ins.Evidence = append(
			ins.Evidence,
			"an active incident is already reported for "+ref.Describe(),
		)
		return
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func (e *Engine) applyFeedbackBias(inc *model.Incident, ins *Insight) {
	if e.feedback == nil || ins.Pattern == "" {
		return
	}
	bias := e.feedback.Bias(feedbackKey(inc, ins.Pattern))
	ins.Confidence = minFloat(ins.Confidence+bias, 1)
	if ins.Confidence < 0 {
		ins.Confidence = 0
	}
}
