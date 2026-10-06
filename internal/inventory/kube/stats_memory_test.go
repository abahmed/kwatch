package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

const mib = 1 << 20

var memoryT0 = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

func attrNumber(attrs map[string]inventory.Value, name string) float64 {
	value, _ := attrs[name].AsNumber()
	return value
}

// noted records a reading on node "a" and returns the history that the
// node's next poll would publish for the container.
func noted(
	log *memoryLog, id inventory.EntityID, started, at time.Time,
	bytes float64,
) map[string]inventory.Value {
	log.note(id, "a", started, at, bytes)
	for _, o := range log.observations("a", at) {
		if o.Entity == id {
			return o.Attributes
		}
	}
	return nil
}

func TestMemoryLogReportsThePeakOfTheLastDay(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	started := memoryT0.Add(-time.Hour)

	noted(log, id, started, memoryT0, 300*mib)
	noted(log, id, started, memoryT0.Add(time.Hour), 610*mib)
	attrs := noted(log, id, started, memoryT0.Add(2*time.Hour), 200*mib)

	assert.Equal(t, 610.0*mib, attrNumber(attrs, AttrMemoryPeak24h))
	assert.NotContains(t, attrs, AttrMemoryPrevPeak, "no run has ended")
}

func TestMemoryLogForgetsAPeakOlderThanADay(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	started := memoryT0.Add(-time.Hour)

	noted(log, id, started, memoryT0, 900*mib)
	attrs := noted(log, id, started, memoryT0.Add(25*time.Hour), 100*mib)

	assert.Equal(t, 100.0*mib, attrNumber(attrs, AttrMemoryPeak24h))
}

func TestMemoryLogDescribesTheRunThatEnded(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	started := memoryT0
	for i, size := range []float64{200, 300, 400, 512} {
		noted(log, id, started, memoryT0.Add(time.Duration(i)*time.Hour),
			size*mib)
	}

	restarted := memoryT0.Add(3*time.Hour + time.Minute)
	attrs := noted(log, id, restarted, restarted, 50*mib)

	assert.Equal(t, 200.0*mib, attrNumber(attrs, AttrMemoryPrevStart))
	assert.Equal(t, 512.0*mib, attrNumber(attrs, AttrMemoryPrevPeak))
	assert.Equal(t, 3*time.Hour.Seconds(),
		attrNumber(attrs, AttrMemoryPrevSeconds))
	assert.Equal(t, 0.0, attrNumber(attrs, AttrMemoryPrevDrawdown),
		"a steady climb never falls")
	assert.True(t, memoryT0.Add(3*time.Hour).Equal(
		attrs[AttrMemoryPrevEnded].AsTime()))
}

func TestMemoryLogMeasuresHowMuchARunFell(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	for i, size := range []float64{400, 100, 400} {
		noted(log, id, memoryT0, memoryT0.Add(time.Duration(i)*time.Minute),
			size*mib)
	}
	restarted := memoryT0.Add(time.Hour)

	attrs := noted(log, id, restarted, restarted, 10*mib)

	assert.InDelta(t, 75.0, attrNumber(attrs, AttrMemoryPrevDrawdown), 0.01)
}

func TestMemoryLogWithoutStartTimeNeverSplitsRuns(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")

	noted(log, id, time.Time{}, memoryT0, 500*mib)
	attrs := noted(log, id, time.Time{}, memoryT0.Add(time.Minute), 10*mib)

	assert.NotContains(t, attrs, AttrMemoryPrevPeak)
}

func TestMemoryLogForgetsContainersNotReadForADay(t *testing.T) {
	log := newMemoryLog()
	id := ContainerID("shop", "api-1", "app")
	noted(log, id, memoryT0, memoryT0, 1)

	log.forgetBefore(memoryT0.Add(time.Minute), nil)

	assert.Empty(t, log.containers)
}
