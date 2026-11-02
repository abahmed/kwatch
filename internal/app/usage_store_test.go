package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// usageNow is the clock of openTestStore.
var usageNow = time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

func TestUsageStoreRoundTripsAndDeletesLeftContainers(t *testing.T) {
	store := newUsageStore(openTestStore(t))
	api := kube.ContainerID("shop", "api-1", "app")
	worker := kube.ContainerID("shop", "worker-1", "app")
	record := kube.UsageRecord{Node: "a", Seen: usageNow,
		Run:   &kube.UsageRun{Samples: 2, Peak: 7},
		Hours: []kube.UsageHour{{Start: usageNow, CPUMilli: 250}}}
	require.NoError(t, store.SaveUsage(map[inventory.EntityID]kube.UsageRecord{
		api: record, worker: record,
	}))
	require.NoError(t, store.SaveUsage(map[inventory.EntityID]kube.UsageRecord{
		api: record,
	}))

	got, err := store.LoadUsage()

	require.NoError(t, err)
	require.Len(t, got, 1, "the container that left was deleted")
	assert.Equal(t, record.Hours, got[api].Hours)
	assert.Equal(t, 7.0, got[api].Run.Peak)
}

func TestUsageStoreSkipsAHistoryPastItsWindow(t *testing.T) {
	store := newUsageStore(openTestStore(t))
	old := kube.ContainerID("shop", "api-1", "app")
	require.NoError(t, store.SaveUsage(map[inventory.EntityID]kube.UsageRecord{
		old: {Seen: usageNow.Add(-25 * time.Hour)},
	}))

	got, err := store.LoadUsage()

	require.NoError(t, err)
	assert.Empty(t, got, "expired with its last reading")
}

func TestUsageStoreWithoutStateFileIsNil(t *testing.T) {
	assert.Nil(t, newUsageStore(nil))
}
