package kube

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

// memoryUsageStore is a UsageStore in memory.
type memoryUsageStore struct {
	records map[inventory.EntityID]UsageRecord
	loadErr error
	saveErr error
	saves   int
}

func (s *memoryUsageStore) LoadUsage() (
	map[inventory.EntityID]UsageRecord, error,
) {
	return s.records, s.loadErr
}

func (s *memoryUsageStore) SaveUsage(
	records map[inventory.EntityID]UsageRecord,
) error {
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.records = records
	return nil
}

// withHistory gives the harness's poller a usage store and a synced
// answer, as the application does.
func (h *sourcesHarness) withHistory(
	store UsageStore, synced *bool,
) *sourcesHarness {
	h.poller.cfg.History = store
	h.poller.cfg.Synced = func() bool { return *synced }
	return h
}

func TestUsageHistorySurvivesARestart(t *testing.T) {
	store := &memoryUsageStore{}
	synced := true
	container := ContainerID("ns", "p", "c")
	first := newSourcesHarness(t, "a").withHistory(store, &synced)
	first.observe(container)
	first.poller.loadUsage()
	first.kubelet.set("a/stats/summary", podWithMemory)
	first.poll()
	first.poller.saveUsage(true)
	require.Contains(t, store.records, container)

	// The next kwatch starts with an empty model: nothing it restored
	// may be dropped before the model has synced.
	synced = false
	second := newSourcesHarness(t, "a").withHistory(store, &synced)
	second.now = first.now
	second.poller.loadUsage()
	second.kubelet.set("a/stats/summary", podWithoutContainer)
	second.poll()
	require.Contains(t, second.poller.memory.containers, container)

	synced = true
	second.observe(container)
	second.poll()
	assert.True(t, second.has(container, AttrMemoryPeak24h),
		"the crashed container's history came back")
	hours := second.poller.UsageHours(container)
	require.Len(t, hours, 1)
	assert.Equal(t, 600000000.0, hours[0].MemoryBytes)
}

func TestUsageHistoryIsSavedAtMostEveryFewMinutes(t *testing.T) {
	store := &memoryUsageStore{}
	synced := true
	h := newSourcesHarness(t, "a").withHistory(store, &synced)
	h.poller.loadUsage()

	h.poller.saveUsage(false)
	h.now = h.now.Add(time.Minute)
	h.poller.saveUsage(false)
	assert.Equal(t, 1, store.saves, "a minute later is too soon")

	h.now = h.now.Add(usageSaveEvery)
	h.poller.saveUsage(false)
	h.poller.saveUsage(true)
	assert.Equal(t, 3, store.saves, "the final save always runs")
}

func TestUsageHistoryIsNotSavedWhenItCouldNotBeLoaded(t *testing.T) {
	kept := map[inventory.EntityID]UsageRecord{
		ContainerID("ns", "p", "c"): {Seen: fixedNow()},
	}
	store := &memoryUsageStore{records: kept,
		loadErr: errors.New("read failed")}
	synced := true
	h := newSourcesHarness(t, "a").withHistory(store, &synced)
	h.poller.loadUsage()

	h.poller.saveUsage(true)

	assert.Zero(t, store.saves, "an empty history must not replace it")
}

func TestUsageHistoryRunSavesWhenItStops(t *testing.T) {
	store := &memoryUsageStore{}
	synced := true
	h := newSourcesHarness(t, "a").withHistory(store, &synced)
	h.kubelet.set("a/stats/summary", podWithMemory)
	h.observe(ContainerID("ns", "p", "c"))
	h.poller.cfg.Interval = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	polled := make(chan struct{}, 1)
	h.poller.cfg.Report = func(StatsRound) { polled <- struct{}{} }

	go func() { h.poller.Run(ctx); close(done) }()
	<-polled
	cancel()
	<-done

	assert.Equal(t, 2, store.saves, "after the first poll and at the end")
	assert.Contains(t, store.records, ContainerID("ns", "p", "c"))
}

func TestUsageHoursOfAnUnknownContainerIsEmpty(t *testing.T) {
	h := newSourcesHarness(t, "a")

	assert.Nil(t, h.poller.UsageHours(ContainerID("ns", "p", "c")))
}

// A save that failed is tried again at the next turn, not a full
// interval later.
func TestUsageHistoryFailedSaveIsRetriedNextTurn(t *testing.T) {
	store := &memoryUsageStore{saveErr: errors.New("disk full")}
	synced := true
	h := newSourcesHarness(t, "a").withHistory(store, &synced)
	h.poller.loadUsage()

	h.poller.saveUsage(false)
	store.saveErr = nil
	h.now = h.now.Add(time.Minute)
	h.poller.saveUsage(false)
	assert.Equal(t, 2, store.saves, "the failed save is retried")
	h.now = h.now.Add(time.Minute)
	h.poller.saveUsage(false)
	assert.Equal(t, 2, store.saves, "a saved history waits again")
}

// A history restored for a container that is gone while kwatch was
// down never brings it back into the model: the model takes usage
// readings only for entities it holds, and once synced the history is
// forgotten.
func TestUsageHistoryRestoredForAGoneContainerStaysOut(
	t *testing.T,
) {
	gone := ContainerID("ns", "old", "c")
	store := &memoryUsageStore{records: map[inventory.EntityID]UsageRecord{
		gone: {Node: "a", Seen: fixedNow(), Hours: []UsageHour{{
			Start: fixedNow().Truncate(time.Hour), MemoryBytes: 1}}},
	}}
	synced := false
	h := newSourcesHarness(t, "a").withHistory(store, &synced)
	h.poller.loadUsage()
	require.Contains(t, h.poller.memory.containers, gone)
	h.kubelet.set("a/stats/summary", podWithoutContainer)
	h.poll()
	assert.False(t, modelHas(h, gone, AttrMemoryPeak24h),
		"published unsynced")

	synced = true
	h.poll()
	assert.False(t, modelHas(h, gone, AttrMemoryPeak24h),
		"published when gone")
	assert.NotContains(t, h.poller.memory.containers, gone)
}

// modelHas reports whether the model holds id with attr.
func modelHas(h *sourcesHarness, id inventory.EntityID, attr string) bool {
	entity, ok := h.model.Entity(id)
	if !ok {
		return false
	}
	_, has := entity.Attributes[attr]
	return has
}

func TestUsageRecordStoresSeenToTheSaveInterval(t *testing.T) {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *memoryHistory {
		return &memoryHistory{
			seen: base.Add(d),
			run: memoryRun{samples: 1, firstAt: base,
				lastAt: base.Add(d)},
		}
	}

	a := at(30 * time.Second).record(base.Add(time.Hour))
	b := at(4 * time.Minute).record(base.Add(time.Hour))

	assert.Equal(t, a.Seen, b.Seen, "polls inside one interval match")
	assert.Equal(t, a.Run.LastAt, b.Run.LastAt)
	assert.Equal(t, base, a.Seen)
	c := at(usageSaveEvery).record(base.Add(time.Hour))
	assert.NotEqual(t, a.Seen, c.Seen)
}
