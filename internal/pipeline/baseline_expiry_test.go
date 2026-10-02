package pipeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// The history writer expires baselines on its own goroutine, between
// saves, so an expiry delete never races a baseline write.
func TestHistoryWriterExpiresGoneWorkloadBaselines(t *testing.T) {
	store := NewIncidentStore(openTempStore(t)).(HistoryStore)
	model := inventory.NewModel(inventory.Options{})
	start := timelineAt(0)
	gone := inventory.CoreID("deployment", "shop", "gone")
	quiet := inventory.CoreID("deployment", "shop", "quiet")
	_, err := model.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: start, Entity: quiet})
	require.NoError(t, err)
	for _, id := range []inventory.EntityID{gone, quiet} {
		model.Baselines().Add(id, inventory.MetricReadySeconds, 4, start)
	}
	now := start.Add(inventory.BaselineTTL + time.Hour)
	clock := func() time.Time { return now }
	w := newHistoryWriter(store, model, wallTimer, &workerStats{})
	w.expireFrom(clock)

	w.flush()
	saved, err := store.LoadBaselines()
	require.NoError(t, err)
	require.Contains(t, saved, gone.String(),
		"expiry waits one interval after start")

	now = now.Add(baselineExpiryInterval)
	w.flush()
	saved, err = store.LoadBaselines()
	require.NoError(t, err)
	assert.NotContains(t, saved, gone.String())
	assert.Contains(t, saved, quiet.String(),
		"a workload that still exists keeps its baseline")

	// A workload recreated after its baseline expired starts a new one
	// that the next write keeps.
	model.Baselines().Add(gone, inventory.MetricReadySeconds, 6, now)
	w.flush()
	saved, err = store.LoadBaselines()
	require.NoError(t, err)
	assert.Contains(t, saved, gone.String())
}

// After kwatch was down for days, the restored restart window starts
// again instead of counting every unwatched hour as zero restarts.
func TestHistoryRecorderRestoresBaselinesAtTheClock(t *testing.T) {
	store := NewIncidentStore(openTempStore(t))
	workload := inventory.CoreID("deployment", "shop", "api")
	start := timelineAt(0)
	before := inventory.NewBaselines()
	before.AddRestarts(workload, 3, start)
	require.NoError(t,
		store.(HistoryStore).SaveBaselines(before.TakeDirty()))

	restart := start.Add(72 * time.Hour)
	model := inventory.NewModel(inventory.Options{})
	recorder := newHistoryRecorder(Dependencies{Model: model,
		Store: store, Clock: &fakeClock{now: restart},
		Timer: wallTimer}, &workerStats{})
	recorder.start()
	defer recorder.stop(nil)
	model.Baselines().AddRestarts(workload, 1, restart.Add(time.Hour))

	stat, ok := model.Baselines().Stat(workload,
		inventory.MetricRestartsPerHour)
	require.True(t, ok)
	assert.Equal(t, []float64{0}, stat.Recent)
}
