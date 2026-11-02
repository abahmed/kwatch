package kube

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestMemoryLogKeepsTheCPUPeakOfEachHour(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")

	log.noteCPU(id, "a", memoryT0, 120)
	log.noteCPU(id, "a", memoryT0.Add(10*time.Minute), 900)
	log.noteCPU(id, "a", memoryT0.Add(time.Hour), 300)
	attrs := noted(log, id, memoryT0, memoryT0.Add(2*time.Hour), 64*mib)

	assert.Equal(t, 900.0, attrNumber(attrs, AttrCPUPeak24h))
	hours := log.containers[id].usageHours(memoryT0.Add(2 * time.Hour))
	require.Len(t, hours, 3)
	assert.Equal(t, memoryT0, hours[0].Start)
	assert.Equal(t, 900.0, hours[0].CPUMilli)
	assert.Equal(t, 300.0, hours[1].CPUMilli)
	assert.Equal(t, 64.0*mib, hours[2].MemoryBytes)
}

func TestMemoryLogForgetsACPUPeakOlderThanADay(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	log.noteCPU(id, "a", memoryT0, 900)
	log.noteCPU(id, "a", memoryT0.Add(25*time.Hour), 100)

	attrs := noted(log, id, memoryT0, memoryT0.Add(25*time.Hour), mib)

	assert.Equal(t, 100.0, attrNumber(attrs, AttrCPUPeak24h))
}

func TestMemoryLogKeepsAContainerReadOnlyForCPU(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	log.noteCPU(id, "a", memoryT0, 50)

	log.forgetBefore(memoryT0.Add(-time.Minute), nil)

	assert.Contains(t, log.containers, id, "a CPU reading is a reading")
}

func TestUsageHistoryRoundTripsThroughItsRecord(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	for i, size := range []float64{200, 400, 300} {
		at := memoryT0.Add(time.Duration(i) * time.Hour)
		log.note(id, "a", memoryT0, at, size*mib)
		log.noteCPU(id, "a", at, float64(100*(i+1)))
	}
	restart := memoryT0.Add(3 * time.Hour)
	log.note(id, "a", restart, restart, 50*mib)
	now := restart.Add(time.Minute)
	want := log.observations("a", now)[0].Attributes

	data, err := json.Marshal(log.snapshot(now)[id])
	require.NoError(t, err)
	var record UsageRecord
	require.NoError(t, json.Unmarshal(data, &record))
	restored := newMemoryLog()
	require.Equal(t, 1, restored.restore(
		map[inventory.EntityID]UsageRecord{id: record}, now))

	got := restored.observations("a", now)
	require.Len(t, got, 1)
	assert.Equal(t, want, got[0].Attributes)
	assert.Equal(t, log.containers[id].usageHours(now),
		restored.containers[id].usageHours(now))
}

func TestUsageHistoryContinuesTheRunAfterARestore(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	log.note(id, "a", memoryT0, memoryT0, 500*mib)
	restored := newMemoryLog()
	restored.restore(log.snapshot(memoryT0), memoryT0)

	attrs := noted(restored, id, memoryT0, memoryT0.Add(time.Minute), mib)

	assert.NotContains(t, attrs, AttrMemoryPrevPeak,
		"the same start time is the same run")
	assert.Equal(t, 500.0*mib, attrNumber(attrs, AttrMemoryPeak24h))
}

func TestUsageHistoryStoresOnlyTheNewestContainers(t *testing.T) {
	log := newMemoryLog()
	for i := range maxUsageRecords + 10 {
		id := ContainerID("ns", fmt.Sprintf("p-%d", i), "c")
		log.noteCPU(id, "a", memoryT0.Add(time.Duration(i)*time.Second), 1)
	}
	old := ContainerID("ns", "old", "c")
	log.noteCPU(old, "a", memoryT0.Add(-25*time.Hour), 1)
	now := memoryT0.Add(time.Hour)

	records := log.snapshot(now)

	assert.Len(t, records, maxUsageRecords)
	assert.NotContains(t, records, ContainerID("ns", "p-0", "c"),
		"the least recently read container is left out")
	assert.Contains(t, records,
		ContainerID("ns", fmt.Sprintf("p-%d", maxUsageRecords+9), "c"))
	assert.NotContains(t, records, old, "outside the window")
}

func TestUsageHistoryRestoreSkipsWhatItCannotUse(t *testing.T) {
	now := memoryT0
	fresh := ContainerID("ns", "fresh", "c")
	records := map[inventory.EntityID]UsageRecord{
		fresh: {Seen: now, Hours: []UsageHour{
			{Start: now, MemoryBytes: math.Inf(1), CPUMilli: -5},
			{Start: now.Add(-30 * time.Hour), MemoryBytes: 9},
			{Start: now.Add(5 * time.Hour), MemoryBytes: 9},
		}},
		ContainerID("ns", "stale", "c"):  {Seen: now.Add(-25 * time.Hour)},
		ContainerID("ns", "future", "c"): {Seen: now.Add(time.Hour)},
		{}:                               {Seen: now},
	}
	log := newMemoryLog()

	assert.Equal(t, 1, log.restore(records, now))
	hours := log.containers[fresh].usageHours(now)
	require.Len(t, hours, 1)
	assert.Zero(t, hours[0].MemoryBytes)
	assert.Zero(t, hours[0].CPUMilli)
}

func TestUsageHistoryRestoreKeepsALiveHistory(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	log.note(id, "a", memoryT0, memoryT0, 10*mib)

	restored := log.restore(map[inventory.EntityID]UsageRecord{
		id: {Seen: memoryT0, Run: &UsageRun{Samples: 1, Peak: 900 * mib}},
	}, memoryT0)

	assert.Zero(t, restored)
	assert.Equal(t, 10.0*mib, log.containers[id].run.peak)
}

func TestUsageRecordWrittenBeforeCPUStillRestores(t *testing.T) {
	// A record with memory only, as written before CPU peaks existed.
	stored := `{"node":"a","seen":"2024-01-01T12:00:00Z",
		"run":{"samples":2,"firstAt":"2024-01-01T11:00:00Z",
		"lastAt":"2024-01-01T12:00:00Z","first":1,"peak":7,"last":7},
		"hours":[{"start":"2024-01-01T12:00:00Z","memoryBytes":7}]}`
	var record UsageRecord
	require.NoError(t, json.Unmarshal([]byte(stored), &record))
	id := ContainerID("shop", "api-1", "app")
	log := newMemoryLog()
	log.restore(map[inventory.EntityID]UsageRecord{id: record}, memoryT0)

	attrs := log.observations("a", memoryT0)[0].Attributes

	assert.Equal(t, 7.0, attrNumber(attrs, AttrMemoryPeak24h))
	assert.NotContains(t, attrs, AttrCPUPeak24h)
}
