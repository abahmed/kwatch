package message

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
)

// caseFactsStory names the measured detail that distinguishes this failure
// from another incident with the same reason.
func caseFactsStory(report *Report) string {
	if report == nil {
		return ""
	}
	facts := report.Facts
	switch report.Reason {
	case constant.ReasonServiceBackendsDegraded:
		return fmt.Sprintf(
			"⚠️ %d/%d selected backend pods are unready; "+
				"the Service still has %d ready endpoints.",
			facts.UnreadyBackendPods, facts.BackendPods,
			facts.HealthyEndpoints,
		)
	case constant.ReasonServiceNoEndpoints:
		if facts.BackendPods > 0 {
			return fmt.Sprintf(
				"🚨 Service has no ready endpoints; %d/%d selected "+
					"backend pods are unready.",
				facts.UnreadyBackendPods, facts.BackendPods,
			)
		}
		if facts.BackendsObserved {
			return "🚨 Service has no ready endpoints; no pods " +
				"currently match its selector."
		}
		return "🚨 Service has no ready endpoints."
	case constant.ReasonDeploymentAvailable,
		constant.ReasonDeploymentUnavailable:
		if facts.DesiredReplicas > 0 {
			return fmt.Sprintf(
				"📊 %d/%d replicas are ready.",
				facts.ReadyReplicas, facts.DesiredReplicas,
			)
		}
	case constant.ReasonServicePortMismatch:
		if facts.MissingServicePortKind != "" &&
			facts.MissingServicePortValue != "" {
			return fmt.Sprintf(
				"🔌 EndpointSlices do not publish %s %q.",
				facts.MissingServicePortKind,
				facts.MissingServicePortValue,
			)
		}
	case constant.ReasonFailedGetResourceMetric,
		constant.ReasonFailedComputeMetricsReplicas,
		constant.ReasonFailedGetMetrics:
		if facts.MetricName != "" &&
			(!causeIsRenderable(report.Diagnosis) ||
				!strings.Contains(diagnosisCause(report), facts.MetricName)) {
			return "📊 Affected metric: " + facts.MetricName + "."
		}
	}
	return ""
}

func diagnosisCause(report *Report) string {
	if report.Diagnosis == nil {
		return ""
	}
	return report.Diagnosis.Cause
}
