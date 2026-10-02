package detectors

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// hpaFailure maps a false HPA condition reason to a finding.
type hpaFailure struct {
	reason  string
	summary string
}

const (
	metricsSummary  = "Autoscaler cannot read the metrics it scales on"
	selectorSummary = "Autoscaler cannot resolve its target's pod selector"
)

// hpaFailures is keyed by the reason the HPA controller sets on its
// ScalingActive and AbleToScale conditions. Every metric-read reason is
// folded into one finding reason because they share a cause.
var hpaFailures = map[string]hpaFailure{
	"FailedGetResourceMetric": {
		reasons.FailedGetResourceMetric, metricsSummary},
	"FailedGetContainerResourceMetric": {
		reasons.FailedGetResourceMetric, metricsSummary},
	"FailedGetPodsMetric": {
		reasons.FailedGetResourceMetric, metricsSummary},
	"FailedGetObjectMetric": {
		reasons.FailedGetResourceMetric, metricsSummary},
	"FailedGetExternalMetric": {
		reasons.FailedGetResourceMetric, metricsSummary},
	"FailedComputeMetricsReplicas": {
		reasons.FailedGetResourceMetric, metricsSummary},
	"InvalidMetricSourceType": {
		reasons.FailedGetResourceMetric, metricsSummary},
	"FailedGetScale": {
		reasons.FailedGetScale,
		"Autoscaler cannot read its scale target"},
	"FailedUpdateScale": {
		reasons.FailedUpdateScale,
		"Autoscaler cannot update its scale target"},
	"SelectorRequired": {reasons.HPAInvalidSelector, selectorSummary},
	"InvalidSelector":  {reasons.HPAInvalidSelector, selectorSummary},
	"AmbiguousSelector": {
		reasons.HPAInvalidSelector, selectorSummary},
}

// hpaFailureFor returns the mapped failure, or a generic HPA failure for
// a reason this map does not know.
func hpaFailureFor(reason, fallbackSummary string) hpaFailure {
	if known, ok := hpaFailures[reason]; ok {
		return known
	}
	return hpaFailure{reasons.HPAScalingError, fallbackSummary}
}

// HPA detects autoscalers that cannot compute or apply a scale.
type HPA struct{}

// Name implements detection.Detector.
func (HPA) Name() string { return "hpa" }

// Kinds implements detection.Detector.
func (HPA) Kinds() []inventory.Kind { return []inventory.Kind{kube.KindHPA} }

// Detect implements detection.Detector.
func (HPA) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	out := hpaConditionFindings(ctx, e)
	current, _ := number(e, kube.AttrCurrentReplicas)
	desired, _ := number(e, kube.AttrDesiredReplicas)
	maximum, ok := number(e, kube.AttrMaxReplicas)
	if ok && current >= maximum && desired >= maximum {
		if status, _, since := condition(e, "ScalingLimited"); status ==
			"True" {
			out = append(out, detection.Finding{
				Reason: reasons.HPAMaxedOut, Severity: detection.Warning,
				Since: since,
				Summary: "Autoscaler is at its maximum of " +
					strconv.Itoa(int(maximum)) + " replicas and wants more",
			})
		}
	}
	return out
}

// hpaConditionFindings reports each false condition once: both conditions
// often carry the same reason (for example FailedGetScale). A condition
// must stay False for DefaultConditionGrace, as other conditions do.
func hpaConditionFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	seen := map[string]bool{}
	add := func(conditionType, fallbackSummary string) {
		status, reason, since := condition(e, conditionType)
		if status != "False" || reason == reasons.ScalingDisabled {
			return
		}
		if !sustained(ctx, "hpa/"+conditionType, since,
			DefaultConditionGrace) {
			return
		}
		failure := hpaFailureFor(reason, fallbackSummary)
		if seen[failure.reason] {
			return
		}
		seen[failure.reason] = true
		out = append(out, detection.Finding{
			Reason: failure.reason, Severity: detection.Warning,
			Since: since, Summary: failure.summary,
			Evidence: []detection.Evidence{{
				Label: "message", Value: conditionMessage(e, conditionType),
			}},
		})
	}
	add("ScalingActive", "Autoscaler is not active")
	add("AbleToScale", "Autoscaler cannot change the replicas")
	return out
}
