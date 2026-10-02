package pipeline

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/metrics"
)

func TestEngineRecordsDecisionLag(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h := newHarness(t, start)
	lag := &metrics.DefaultRegistry().DecisionLag
	count, sum := lag.Count(), lag.Sum()

	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	h.now = h.now.Add(1500 * time.Millisecond)
	h.engine.step(context.Background(), h.now, h.checks)

	if got := lag.Count() - count; got != 1 {
		t.Fatalf("observations = %d, want 1", got)
	}
	if got := lag.Sum() - sum; got < 1.49 || got > 1.51 {
		t.Fatalf("lag = %vs, want 1.5s", got)
	}

	// An idle iteration has nothing to time.
	h.now = h.now.Add(time.Second)
	h.engine.step(context.Background(), h.now, h.checks)
	if got := lag.Count() - count; got != 1 {
		t.Fatalf("idle step recorded a lag: observations = %d", got)
	}
}

// lagBursts is how many bursts the freshness test times; the p99 of 100
// samples is the second slowest.
const lagBursts = 100

// maxDecisionLagP99 is the freshness goal of docs/production-goals.md.
const maxDecisionLagP99 = 2 * time.Second

// TestEngineDecisionLagAt5000Pods checks the in-process part of
// freshness at the documented scale: 5,000 pods in 1,000 Deployments on
// 50 nodes, 1,000 of the pods failing. Each burst submits a new status
// for every failing pod at once, as an informer relist would, and the
// test times, on the wall clock, Submit plus the loop iteration that
// applies the burst to the model, evaluates the detectors, solves root
// causes and applies the incident decisions. That is the lag
// kwatch_pipeline_decision_lag_seconds reports, minus the wait for the
// loop to wake. API server and informer delays are outside it.
func TestEngineDecisionLagAt5000Pods(t *testing.T) {
	// The p99 budget is a wall-clock number for a production build:
	// under -race it measures the detector, not the engine. The normal
	// test run still enforces it.
	if testing.Short() {
		t.Skip("builds a 5,000-pod cluster")
	}
	if raceEnabled {
		t.Skip("wall-clock budget; not meaningful under -race")
	}
	c := newBenchCluster(t)
	ctx := context.Background()
	lags := make([]time.Duration, 0, lagBursts)
	for i := range lagBursts {
		began := time.Now()
		c.h.engine.Submit(ctx, c.updates[i%2]...)
		c.h.now = c.h.now.Add(time.Second)
		c.h.engine.step(ctx, c.h.now, c.h.checks)
		lags = append(lags, time.Since(began))
	}
	sort.Slice(lags, func(i, j int) bool { return lags[i] < lags[j] })
	p99 := lags[len(lags)*99/100-1]
	t.Logf("decision lag over %d bursts of %d observations: "+
		"p50 %v, p99 %v, max %v", lagBursts, len(c.updates[0]),
		lags[len(lags)/2], p99, lags[len(lags)-1])
	if p99 > maxDecisionLagP99 {
		t.Fatalf("p99 decision lag %v exceeds %v", p99,
			maxDecisionLagP99)
	}
}
