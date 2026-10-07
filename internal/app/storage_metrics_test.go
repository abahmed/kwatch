package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/storage"
)

func TestStorageMetricsPublishAddsDeltasOnly(t *testing.T) {
	registry := &metrics.Registry{}
	m := newStorageMetrics(registry)

	m.publish(storage.Stats{CorruptRecords: 2, Expired: 5, Evicted: 1})
	m.publish(storage.Stats{CorruptRecords: 3, Expired: 5, Evicted: 4})

	assert.Equal(t, int64(3), registry.StorageCorruptRecords.Load())
	assert.Equal(t, int64(5), registry.StorageExpired.Load())
	assert.Equal(t, int64(4), registry.StorageEvicted.Load())
}

func TestStorageMetricsPublishUntilReturnsOnCancel(t *testing.T) {
	registry := &metrics.Registry{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats := func() storage.Stats { return storage.Stats{Evicted: 7} }

	newStorageMetrics(registry).publishUntil(
		ctx, stats, make(chan storage.PassResult))

	assert.Equal(t, int64(7), registry.StorageEvicted.Load())
}

func TestRecordStoreResetCountsBoundedReason(t *testing.T) {
	registry := &metrics.Registry{}

	recordStoreReset(registry, storage.Reset{
		Reason: storage.ResetSchemaMismatch})
	recordStoreReset(registry, storage.Reset{Reason: "bogus"})

	assert.Equal(t, int64(1), registry.StorageResets[0].Load())
	assert.Equal(t, int64(1), registry.StorageResets[1].Load())
}

func TestStorageMetricsReportsOverCapOnlyOnChange(t *testing.T) {
	var reports []bool
	m := newStorageMetrics(&metrics.Registry{})
	m.overCap = func(over bool) { reports = append(reports, over) }

	m.publish(storage.Stats{})
	m.publish(storage.Stats{OverCap: true})
	m.publish(storage.Stats{OverCap: true})
	m.publish(storage.Stats{})

	assert.Equal(t, []bool{true, false}, reports)
}

func TestReportOverCapShowsDegradedStoreSize(t *testing.T) {
	deps := componentDeps()
	report := reportOverCap(deps)

	report(true)
	over := deps.healthServer.ComponentStatuses()[storeSizeComponent]
	report(false)
	under := deps.healthServer.ComponentStatuses()[storeSizeComponent]

	assert.Equal(t, "degraded", over.State)
	assert.Equal(t, "storage_over_cap", over.Reason)
	assert.True(t, over.Available, "over the cap never blocks readiness")
	assert.Equal(t, "running", under.State)
}

// The compactor restarts with the same metrics, so lifetime totals are
// not counted twice.
func TestStorageMetricsSharedAcrossRestartsCountsOnce(t *testing.T) {
	registry := &metrics.Registry{}
	m := newStorageMetrics(registry)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stats := func() storage.Stats { return storage.Stats{Evicted: 7} }

	m.publishUntil(ctx, stats, make(chan storage.PassResult))
	m.publishUntil(ctx, stats, make(chan storage.PassResult))

	assert.Equal(t, int64(7), registry.StorageEvicted.Load())
}

func TestStorageMetricsPublishFileSizeAndRewrites(t *testing.T) {
	registry := &metrics.Registry{}
	m := newStorageMetrics(registry)

	m.publish(storage.Stats{FileBytes: 900, FreeBytes: 700, Rewrites: 1})
	m.publish(storage.Stats{FileBytes: 100, FreeBytes: 10, Rewrites: 2})

	assert.Equal(t, int64(100), registry.StorageFileBytes.Load())
	assert.Equal(t, int64(10), registry.StorageFreeBytes.Load())
	assert.Equal(t, int64(2), registry.StorageRewrites.Load())
}
