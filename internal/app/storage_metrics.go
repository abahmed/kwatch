package app

import (
	"context"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/storage"
)

// storageMetrics publishes the store's lifetime counters as deltas, so the
// registry counters only ever increase. It also reports when the store
// goes over or comes back under its size cap.
type storageMetrics struct {
	registry *metrics.Registry
	last     storage.Stats
	// overCap, when set, is called each time Stats.OverCap changes.
	overCap func(bool)
}

func newStorageMetrics(registry *metrics.Registry) *storageMetrics {
	return &storageMetrics{registry: registry}
}

// storeSizeComponent is the /health component for the store's size cap.
// It is separate from "state-store", whose reset reason stays for the
// whole session.
const storeSizeComponent = "state-store-size"

// reportOverCap shows on /health that pinned data keeps the store over
// its size cap. The store keeps working, so readiness is not affected.
func reportOverCap(deps *serverDeps) func(bool) {
	return func(over bool) {
		if deps.healthServer == nil {
			return
		}
		if over {
			deps.healthServer.SetComponentStatus(storeSizeComponent,
				"degraded", "storage_over_cap", true)
			return
		}
		deps.healthServer.SetComponentStatus(storeSizeComponent,
			"running", "", true)
	}
}

// publish adds what the store counted since the previous call.
func (m *storageMetrics) publish(now storage.Stats) {
	m.registry.StorageCorruptRecords.Add(
		int64(now.CorruptRecords - m.last.CorruptRecords))
	m.registry.StorageExpired.Add(int64(now.Expired - m.last.Expired))
	m.registry.StorageEvicted.Add(int64(now.Evicted - m.last.Evicted))
	m.registry.StorageRewrites.Add(int64(now.Rewrites - m.last.Rewrites))
	m.registry.StorageFileBytes.Store(now.FileBytes)
	m.registry.StorageFreeBytes.Store(now.FreeBytes)
	if m.overCap != nil && now.OverCap != m.last.OverCap {
		m.overCap(now.OverCap)
	}
	m.last = now
}

// publishUntil publishes after each compactor pass and once more when ctx
// ends, then returns. The pass channel is lossy, which is safe because
// publish works on totals.
func (m *storageMetrics) publishUntil(
	ctx context.Context, stats func() storage.Stats,
	passes <-chan storage.PassResult,
) {
	for {
		select {
		case <-passes:
			m.publish(stats())
		case <-ctx.Done():
			m.publish(stats())
			return
		}
	}
}

// recordStoreReset counts a reset at open with its bounded reason.
func recordStoreReset(registry *metrics.Registry, reset storage.Reset) {
	registry.IncStorageReset(reset.Reason)
}
