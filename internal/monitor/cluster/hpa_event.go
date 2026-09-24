package cluster

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

var (
	metricNamePattern = regexp.MustCompile(
		`(?i)(?:resource|external metric|metric)[ /\"]+([a-zA-Z0-9_.-]+)`,
	)
	missingRequestPattern = regexp.MustCompile(
		`(?i)missing request for ([a-zA-Z0-9_.-]+) in container ` +
			`([^ ]+) of pod ([^ ,:]+)`,
	)
)

func hpaEventFacts(reason, message string) model.Facts {
	switch reason {
	case constant.ReasonFailedGetResourceMetric,
		constant.ReasonFailedComputeMetricsReplicas,
		constant.ReasonFailedGetMetrics:
	default:
		return model.Facts{}
	}

	lower := strings.ToLower(message)
	facts := model.Facts{}
	match := missingRequestPattern.FindStringSubmatch(message)
	if len(match) == 4 {
		facts.MetricFailure = "missing_request"
		facts.MetricName = match[1]
		facts.MetricContainer = match[2]
		facts.MetricPod = match[3]
		return facts
	}
	switch {
	case containsAny(lower,
		"server currently unable to handle the request",
		"no known available metric versions found",
		"metrics api is unavailable",
		"service unavailable",
	):
		facts.MetricFailure = "api_unavailable"
	case containsAny(lower,
		"no metrics returned",
		"did not receive metrics for any ready pods",
		"missing metrics for pod",
	):
		facts.MetricFailure = "missing_pod_metrics"
	case containsAny(lower,
		"invalid metric",
		"invalid selector",
		"failed to parse",
	):
		facts.MetricFailure = "invalid_metric"
	}
	if match := metricNamePattern.FindStringSubmatch(message); len(match) == 2 {
		facts.MetricName = match[1]
	}
	return facts
}
