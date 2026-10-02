package metrics

import (
	"math"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// decisionLagBuckets are the fixed upper bounds, in seconds, of
// kwatch_pipeline_decision_lag_seconds. The goal is a decision within
// about one second, so the buckets are dense below it and stop at ten
// seconds; anything slower lands in +Inf.
var decisionLagBuckets = [...]float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10,
}

var decisionLagDesc = prometheus.NewDesc(
	"kwatch_pipeline_decision_lag_seconds",
	"Time from an observation batch being submitted to the pipeline "+
		"until its decisions are applied", nil, nil)

// LagHistogram is a fixed-bucket histogram kept in atomics so the
// decision loop can record into it without a lock. counts[i] holds the
// observations at or below decisionLagBuckets[i] that were above the
// previous bound; the last slot is +Inf.
type LagHistogram struct {
	counts  [len(decisionLagBuckets) + 1]atomic.Uint64
	count   atomic.Uint64
	sumBits atomic.Uint64
}

// observe records one value in seconds. Negative values count as zero.
func (h *LagHistogram) observe(seconds float64) {
	seconds = max(seconds, 0)
	slot := len(decisionLagBuckets)
	for i, bound := range decisionLagBuckets {
		if seconds <= bound {
			slot = i
			break
		}
	}
	h.counts[slot].Add(1)
	h.count.Add(1)
	for {
		old := h.sumBits.Load()
		sum := math.Float64frombits(old) + seconds
		if h.sumBits.CompareAndSwap(old, math.Float64bits(sum)) {
			return
		}
	}
}

// Count is the number of observations recorded.
func (h *LagHistogram) Count() uint64 { return h.count.Load() }

// Sum is the total of every observation, in seconds.
func (h *LagHistogram) Sum() float64 {
	return math.Float64frombits(h.sumBits.Load())
}

// metric renders the histogram with cumulative bucket counts.
func (h *LagHistogram) metric() prometheus.Metric {
	buckets := make(map[float64]uint64, len(decisionLagBuckets))
	var cumulative uint64
	for i, bound := range decisionLagBuckets {
		cumulative += h.counts[i].Load()
		buckets[bound] = cumulative
	}
	return prometheus.MustNewConstHistogram(decisionLagDesc,
		h.Count(), h.Sum(), buckets)
}

// ObserveDecisionLag records how long one observation batch waited
// from submission until the pipeline applied its decisions.
func (r *Registry) ObserveDecisionLag(seconds float64) {
	r.DecisionLag.observe(seconds)
}
