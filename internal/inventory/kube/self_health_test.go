package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestSelfHealthClockFiresEveryHalfHour(t *testing.T) {
	var clock selfHealthClock
	now := fixedNow()

	assert.True(t, clock.due(now), "the first line says where it started")
	assert.False(t, clock.due(now.Add(29*time.Minute)))
	assert.True(t, clock.due(now.Add(30*time.Minute)))
	assert.False(t, clock.due(now.Add(31*time.Minute)))
}

func TestSelfHealthFieldsNameTheRuntimeAndTheBoundedMaps(t *testing.T) {
	r := newReachPoller()
	r.pollAfter(0, true)

	fields := r.poller.selfHealthFields()

	values := map[string]any{}
	for i := 0; i+1 < len(fields); i += 2 {
		values[fields[i].(string)] = fields[i+1]
	}
	assert.Contains(t, values, "heapAllocMiB")
	assert.Contains(t, values, "heapInuseMiB")
	assert.Greater(t, values["goroutines"], 0)
	assert.Equal(t, 1, values["reachLogNodes"])
	assert.Equal(t, 0, values["memoryLogContainers"])
}

func TestCrashLogSelfHealthCountsItsState(t *testing.T) {
	round := NewCrashLogRound(CrashLogConfig{})
	round.read[inventory.CoreID(KindContainer, "a", "p/c")] = crashRun{}

	assert.Equal(t, []any{"crashLogRuns", 1, "crashLogBackoffs", 0},
		round.selfHealthFields())
}
