package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryShapeCountsPeakHoursAndRunEnds(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	// Three runs, each killed near 510Mi, an hour apart.
	noted(log, id, memoryT0, memoryT0, 200*mib)
	for i := range 3 {
		started := memoryT0.Add(time.Duration(i) * time.Hour)
		noted(log, id, started, started.Add(time.Minute), 200*mib)
		noted(log, id, started, started.Add(50*time.Minute),
			float64(505+i*3)*mib)
	}
	attrs := noted(log, id, memoryT0.Add(3*time.Hour),
		memoryT0.Add(3*time.Hour+time.Minute), 100*mib)

	assert.True(t, memoryT0.Equal(attrs[AttrMemoryHistorySince].AsTime()),
		"the history starts at the first hour read, not 24h ago")
	assert.Equal(t, 3.0, attrNumber(attrs, AttrMemoryPeakHours))
	assert.Equal(t, 3.0, attrNumber(attrs, AttrMemoryRunsEnded))
	assert.Equal(t, 505.0*mib, attrNumber(attrs, AttrMemoryRunsLow))
	assert.Equal(t, 511.0*mib, attrNumber(attrs, AttrMemoryRunsHigh))
}

func TestMemoryShapeWithoutRunsEndedSaysNothingOfThem(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	attrs := noted(log, id, memoryT0, memoryT0, 300*mib)
	assert.NotContains(t, attrs, AttrMemoryRunsEnded)
	assert.Equal(t, 1.0, attrNumber(attrs, AttrMemoryPeakHours))
}

func TestMemoryShapeEndsSurviveRestore(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	noted(log, id, memoryT0, memoryT0, 400*mib)
	noted(log, id, memoryT0.Add(time.Hour), memoryT0.Add(time.Hour),
		100*mib)
	now := memoryT0.Add(time.Hour)
	records := log.snapshot(now)
	require.Len(t, records[id].Ends, 1)

	restored := newMemoryLog()
	restored.restore(records, now)
	attrs := restored.containers[id].attributes(now)
	assert.Equal(t, 1.0, attrNumber(attrs, AttrMemoryRunsEnded))
	assert.Equal(t, 400.0*mib, attrNumber(attrs, AttrMemoryRunsHigh))
}
