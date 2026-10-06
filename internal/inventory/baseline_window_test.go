package inventory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatDropsSamplesOlderThanTheWindow(t *testing.T) {
	b := NewBaselines()
	b.Add(baselineWorkload, MetricReadySeconds, 500, testTime)
	for i := 0; i < 12; i++ {
		b.Add(baselineWorkload, MetricReadySeconds, 20,
			testTime.Add(BaselineWindow+time.Duration(i)*time.Minute))
	}
	stat, _ := b.Stat(baselineWorkload, MetricReadySeconds)
	assert.Len(t, stat.Recent, 12, "the week-old sample is dropped")
	assert.Equal(t, 20.0, stat.High())
}

func TestStatRobustRange(t *testing.T) {
	s := Stat{Recent: []float64{2, 2, 2, 3, 2, 2, 2, 40, 2, 2}}
	assert.Equal(t, 2.0, s.Median())
	assert.Equal(t, 0.0, s.MAD(), "one wild hour does not widen it")
	assert.GreaterOrEqual(t, s.High(), 2.0)
	assert.Equal(t, 2.0, s.Typical())

	quiet := Stat{Recent: []float64{0, 0, 0, 0, 1, 0, 0, 0, 0, 0}}
	assert.Equal(t, 0.0, quiet.Median())
	assert.InDelta(t, 0.1, quiet.Typical(), 0.001,
		"a mostly-zero series is quoted by its mean")
}

// quietHours gives workload hours restarts-per-hour samples of rate.
func quietHours(b *Baselines, hours int, rate int) time.Time {
	at := testTime.Truncate(time.Hour)
	b.Touch(baselineWorkload, at)
	for h := 1; h <= hours; h++ {
		at = testTime.Truncate(time.Hour).Add(time.Duration(h) * time.Hour)
		b.AddRestarts(baselineWorkload, rate, at)
	}
	return at.Add(time.Hour)
}

func TestCompareJudgesAgainstTheWorkloadsOwnNormal(t *testing.T) {
	stable := NewBaselines()
	now := quietHours(stable, 12, 0)
	cmp := stable.Compare(baselineWorkload, MetricRestartsPerHour, 6)
	require.True(t, cmp.Known)
	assert.True(t, cmp.Unusual)
	assert.False(t, cmp.Usual)

	batch := NewBaselines()
	quietHours(batch, 12, 2)
	cmp = batch.Compare(baselineWorkload, MetricRestartsPerHour, 2)
	require.True(t, cmp.Known)
	assert.True(t, cmp.Usual)
	assert.False(t, cmp.Unusual)
	assert.Equal(t, 2.0, cmp.Typical)

	young := NewBaselines()
	quietHours(young, MinBaselineSamples-3, 0)
	assert.False(t, young.Compare(baselineWorkload,
		MetricRestartsPerHour, 50).Known, "too little history")
	_ = now
}

func TestTouchCountsQuietHoursAsQuiet(t *testing.T) {
	b := NewBaselines()
	b.Touch(baselineWorkload, testTime)
	b.AddRestarts(baselineWorkload, 5, testTime.Add(12*time.Hour))
	stat, ok := b.Stat(baselineWorkload, MetricRestartsPerHour)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(stat.Recent), 11)
	assert.Equal(t, 0.0, stat.High())
	assert.Nil(t, restoredCopy(b).TakeDirty(),
		"a restored baseline is clean")
}

func restoredCopy(b *Baselines) *Baselines {
	out := NewBaselines()
	out.Restore(b.TakeDirty(), testTime.Add(12*time.Hour))
	return out
}

func TestRestartsInLastHour(t *testing.T) {
	b := NewBaselines()
	b.AddRestarts(baselineWorkload, 2, testTime)
	b.AddRestarts(baselineWorkload, 3, testTime.Add(30*time.Minute))
	assert.Equal(t, 5.0,
		b.RestartsInLastHour(baselineWorkload, testTime.Add(45*time.Minute)))
	assert.Equal(t, 3.0,
		b.RestartsInLastHour(baselineWorkload, testTime.Add(70*time.Minute)))
}

func TestMemoryPeakIsOneSamplePerHour(t *testing.T) {
	b := NewBaselines()
	hour := testTime.Truncate(time.Hour)
	b.AddMemory(baselineWorkload, 300<<20, hour.Add(time.Minute))
	b.AddMemory(baselineWorkload, 500<<20, hour.Add(20*time.Minute))
	b.AddMemory(baselineWorkload, 400<<20, hour.Add(40*time.Minute))
	b.AddMemory(baselineWorkload, 100<<20, hour.Add(time.Hour+time.Minute))
	stat, ok := b.Stat(baselineWorkload, MetricMemoryPeak)
	require.True(t, ok)
	assert.Equal(t, []float64{500 << 20}, stat.Recent)
}

func TestRebaseStartsMemoryAgainButKeepsRestartsAndReadiness(t *testing.T) {
	b := NewBaselines()
	hour := testTime.Truncate(time.Hour)
	b.Rebase(baselineWorkload, "old", hour)
	b.AddMemory(baselineWorkload, 300<<20, hour)
	b.AddRestarts(baselineWorkload, 1, hour)
	b.Add(baselineWorkload, MetricReadySeconds, 20, hour)
	b.AddRestarts(baselineWorkload, 0, hour.Add(2*time.Hour))
	b.AddMemory(baselineWorkload, 300<<20, hour.Add(2*time.Hour))
	b.AddMemory(baselineWorkload, 300<<20, hour.Add(3*time.Hour))
	_, ok := b.Stat(baselineWorkload, MetricMemoryPeak)
	require.True(t, ok)

	b.Rebase(baselineWorkload, "new", hour.Add(4*time.Hour))
	_, ok = b.Stat(baselineWorkload, MetricMemoryPeak)
	assert.False(t, ok, "new code has new memory")
	_, ok = b.Stat(baselineWorkload, MetricRestartsPerHour)
	assert.True(t, ok)
	_, ok = b.Stat(baselineWorkload, MetricReadySeconds)
	assert.True(t, ok)
}

func TestWarningReasonsAreBounded(t *testing.T) {
	b := NewBaselines()
	hour := testTime.Truncate(time.Hour)
	for i := 0; i < maxWarningReasons+5; i++ {
		b.AddWarning(baselineWorkload, "Reason"+itoaForTest(i), 1, hour)
	}
	b.AddWarning(baselineWorkload, "Reasona", 2, hour.Add(time.Hour))
	b.AddWarning(baselineWorkload, "Reasona", 1, hour.Add(2*time.Hour))
	saved := b.TakeDirty()[baselineWorkload.String()]
	learned := 0
	for metric := range saved.Metrics {
		if isWarningMetric(metric) {
			learned++
		}
	}
	assert.LessOrEqual(t, learned, maxWarningReasons)
	stat, ok := b.Stat(baselineWorkload, WarningMetric("Reasona"))
	require.True(t, ok)
	assert.Equal(t, []float64{1, 2}, stat.Recent)
}

func itoaForTest(i int) string { return string(rune('a' + i)) }
