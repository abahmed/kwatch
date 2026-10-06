package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hist(counts ...float64) histogram {
	return histogram{
		bounds: []float64{0.1, 1, 10}, counts: counts,
		count: counts[len(counts)-1],
	}
}

func TestHistogramQuantileInterpolatesInsideTheBucket(t *testing.T) {
	// 90 calls under 0.1s, 10 more between 1s and 10s.
	h := hist(90, 90, 100)

	p99, ok := h.quantile(0.99)

	require.True(t, ok)
	assert.InDelta(t, 1+9*0.9, p99, 0.001)
	p50, _ := h.quantile(0.5)
	assert.InDelta(t, 0.1*50.0/90, p50, 0.001)
}

func TestHistogramQuantilePastTheLastBucketIsItsLimit(t *testing.T) {
	h := hist(1, 1, 1)
	h.count = 100

	p99, ok := h.quantile(0.99)

	require.True(t, ok)
	assert.Equal(t, 10.0, p99)
}

func TestHistogramQuantileNeedsObservations(t *testing.T) {
	_, ok := hist(0, 0, 0).quantile(0.99)
	assert.False(t, ok)
}

func TestHistogramSinceKeepsOnlyTheNewCalls(t *testing.T) {
	before := hist(50, 50, 50)
	now := hist(150, 150, 150)

	delta, ok := now.since(before)

	require.True(t, ok)
	assert.Equal(t, 100.0, delta.count)
	assert.Equal(t, []float64{100, 100, 100}, delta.counts)
}

func TestHistogramSinceRefusesACounterThatWentBack(t *testing.T) {
	_, ok := hist(10, 10, 10).since(hist(50, 50, 50))
	assert.False(t, ok, "a restart resets every bucket")
}

func TestHistogramSinceRefusesOtherBuckets(t *testing.T) {
	other := histogram{bounds: []float64{1, 5}, counts: []float64{1, 1},
		count: 1}
	_, ok := hist(1, 1, 1).since(other)
	assert.False(t, ok)
}

func TestHistogramMergeAddsCalls(t *testing.T) {
	merged := hist(1, 2, 3).merge(hist(10, 20, 30))
	assert.Equal(t, []float64{11, 22, 33}, merged.counts)
	assert.Equal(t, 33.0, merged.count)
	assert.Equal(t, hist(1, 2, 3), histogram{}.merge(hist(1, 2, 3)))
}
