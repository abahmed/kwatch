package metrics

import (
	"strings"
	"testing"
)

func TestDecisionLagHistogramUsesFixedBuckets(t *testing.T) {
	r := &Registry{}
	for _, seconds := range []float64{0.0005, 0.3, 1.5, 30, -1} {
		r.ObserveDecisionLag(seconds)
	}
	if r.DecisionLag.Count() != 5 {
		t.Fatalf("count = %d, want 5", r.DecisionLag.Count())
	}
	if sum := r.DecisionLag.Sum(); sum < 31.8 || sum > 31.81 {
		t.Fatalf("sum = %v, want 31.8005", sum)
	}
	body := scrape(t, r)
	for _, line := range []string{
		"# TYPE kwatch_pipeline_decision_lag_seconds histogram",
		`kwatch_pipeline_decision_lag_seconds_bucket{le="0.001"} 2`,
		`kwatch_pipeline_decision_lag_seconds_bucket{le="0.5"} 3`,
		`kwatch_pipeline_decision_lag_seconds_bucket{le="2"} 4`,
		`kwatch_pipeline_decision_lag_seconds_bucket{le="10"} 4`,
		`kwatch_pipeline_decision_lag_seconds_bucket{le="+Inf"} 5`,
		"kwatch_pipeline_decision_lag_seconds_count 5",
	} {
		if !strings.Contains(body, line) {
			t.Fatalf("metrics output is missing %q", line)
		}
	}
}
