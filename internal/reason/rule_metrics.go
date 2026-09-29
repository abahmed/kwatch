package reason

import (
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/signal"
)

// metricsAPIs are the aggregated APIs autoscalers read.
var metricsAPIs = []string{
	"v1beta1.metrics.k8s.io", "v1beta1.custom.metrics.k8s.io",
	"v1beta1.external.metrics.k8s.io",
}

// MetricsAPIRule explains autoscalers that cannot read metrics by an
// unavailable metrics API, so every affected HPA joins one problem.
type MetricsAPIRule struct{}

// Name implements Rule.
func (MetricsAPIRule) Name() string { return "metrics-api" }

// Explain implements Rule.
func (MetricsAPIRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	if symptom.Reason != constant.ReasonFailedGetResourceMetric {
		return nil
	}
	text := strings.ToLower(evidenceText(symptom))
	var out []Hypothesis
	for _, name := range metricsAPIs {
		api := knowledge.NewEntityID("apiservice", "", name)
		apiSignals := q.Signals.Active(api)
		if len(apiSignals) == 0 {
			continue
		}
		s := newScorer(0.5)
		s.support(0.2, "the metrics API is unavailable")
		if strings.Contains(text, strings.TrimPrefix(name, "v1beta1.")) {
			s.support(0.15, "the autoscaler error names it")
		}
		score, points := s.result()
		out = append(out, Hypothesis{
			Root: api, RootSignals: apiSignals,
			Chain:   []knowledge.EntityID{api, symptom.Entity},
			Summary: "metrics API " + name + " is unavailable",
			Points:  points, Score: score,
		})
	}
	return out
}
