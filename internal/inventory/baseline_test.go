package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var baselineWorkload = CoreID("deployment", "shop", "api")

func TestBaselineQuantilesAndMean(t *testing.T) {
	b := NewBaselines()
	for i := 1; i <= 100; i++ {
		b.Add(baselineWorkload, MetricReadySeconds, float64(i), testTime)
	}
	stat, ok := b.Stat(baselineWorkload, MetricReadySeconds)
	require.True(t, ok)
	assert.Equal(t, 100, stat.Count)
	assert.Len(t, stat.Recent, sketchSize)
	assert.Equal(t, 68.0, stat.Quantile(0.5))
	assert.Equal(t, 97.0, stat.Quantile(0.95))
	assert.InDelta(t, 96, stat.Mean, 1)
}

func TestBaselineIsUnusual(t *testing.T) {
	tests := []struct {
		name    string
		samples int
		value   float64
		want    bool
	}{
		{"too little history", MinBaselineSamples - 1, 1000, false},
		{"typical", 20, 22, false},
		{"above p95 but under floor", 20, 45, false},
		{"far above", 20, 120, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBaselines()
			for i := 0; i < tt.samples; i++ {
				b.Add(baselineWorkload, MetricReadySeconds,
					float64(18+i%5), testTime)
			}
			assert.Equal(t, tt.want,
				b.IsUnusual(baselineWorkload, MetricReadySeconds, tt.value))
		})
	}
}

func TestBaselineRestartsPerHour(t *testing.T) {
	b := NewBaselines()
	b.AddRestarts(baselineWorkload, 2, testTime)
	b.AddRestarts(baselineWorkload, 1, testTime.Add(10*time.Minute))
	_, ok := b.Stat(baselineWorkload, MetricRestartsPerHour)
	assert.False(t, ok, "the first hour is still open")

	b.AddRestarts(baselineWorkload, 0, testTime.Add(3*time.Hour))
	stat, ok := b.Stat(baselineWorkload, MetricRestartsPerHour)
	require.True(t, ok)
	assert.Equal(t, []float64{3, 0, 0}, stat.Recent)
}

func TestBaselineDirtyAndRestore(t *testing.T) {
	b := NewBaselines()
	assert.Nil(t, b.TakeDirty())
	b.Add(baselineWorkload, MetricJobSeconds, 30, testTime)
	dirty := b.TakeDirty()
	require.Contains(t, dirty, baselineWorkload.String())
	assert.Nil(t, b.TakeDirty())

	restored := NewBaselines()
	restored.Restore(dirty, testTime)
	assert.Nil(t, restored.TakeDirty())
	stat, ok := restored.Stat(baselineWorkload, MetricJobSeconds)
	require.True(t, ok)
	assert.Equal(t, 30.0, stat.Mean)
}

func TestBaselineRejectsInvalidSamples(t *testing.T) {
	b := NewBaselines()
	b.Add(EntityID{}, MetricJobSeconds, 1, testTime)
	b.Add(baselineWorkload, MetricJobSeconds, -1, testTime)
	assert.Nil(t, b.TakeDirty())
}

func TestBaselineExpiredForgetsUnseenWorkloads(t *testing.T) {
	b := NewBaselines()
	gone := CoreID("deployment", "shop", "old")
	b.Add(gone, MetricJobSeconds, 30, testTime)
	b.Add(baselineWorkload, MetricJobSeconds, 30,
		testTime.Add(BaselineTTL))

	assert.Empty(t, b.Expired(testTime.Add(BaselineTTL), nil),
		"a baseline exactly at the TTL is kept")
	removed := b.Expired(testTime.Add(BaselineTTL+time.Second), nil)

	assert.Equal(t, []string{gone.String()}, removed)
	_, ok := b.Stat(gone, MetricJobSeconds)
	assert.False(t, ok)
	_, ok = b.Stat(baselineWorkload, MetricJobSeconds)
	assert.True(t, ok)
	assert.NotContains(t, b.TakeDirty(), gone.String(),
		"an expired baseline must not be written back")
}

func TestBaselineExpiredKeepsExistingWorkloads(t *testing.T) {
	b := NewBaselines()
	quiet := CoreID("deployment", "shop", "quiet")
	gone := CoreID("deployment", "shop", "gone")
	b.Add(quiet, MetricReadySeconds, 5, testTime)
	b.Add(gone, MetricReadySeconds, 5, testTime)
	exists := func(id EntityID) bool { return id == quiet }

	removed := b.Expired(testTime.Add(2*BaselineTTL), exists)

	assert.Equal(t, []string{gone.String()}, removed)
	_, ok := b.Stat(quiet, MetricReadySeconds)
	assert.True(t, ok, "a quiet workload that still exists keeps its "+
		"baseline")
}

func TestBaselineRestoreAfterDowntimeResetsTheHour(t *testing.T) {
	b := NewBaselines()
	b.AddRestarts(baselineWorkload, 2, testTime)
	saved := b.TakeDirty()

	// kwatch was down for two days.
	restart := testTime.Add(48 * time.Hour)
	restored := NewBaselines()
	restored.Restore(saved, restart)
	restored.AddRestarts(baselineWorkload, 1, restart.Add(time.Hour))

	stat, ok := restored.Stat(baselineWorkload, MetricRestartsPerHour)
	require.True(t, ok)
	assert.Equal(t, []float64{0}, stat.Recent,
		"one watched hour, not 48 unwatched zeros")
}

func TestBaselineRestoreAfterShortGapKeepsTheHour(t *testing.T) {
	b := NewBaselines()
	b.AddRestarts(baselineWorkload, 2, testTime)
	saved := b.TakeDirty()

	restored := NewBaselines()
	restored.Restore(saved, testTime.Add(30*time.Minute))
	restored.AddRestarts(baselineWorkload, 1,
		testTime.Truncate(time.Hour).Add(2*time.Hour))

	stat, ok := restored.Stat(baselineWorkload, MetricRestartsPerHour)
	require.True(t, ok)
	assert.Equal(t, []float64{2, 0}, stat.Recent)
}
