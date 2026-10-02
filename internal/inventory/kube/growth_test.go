package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestGrowthTrackerNeedsEnoughSamples(t *testing.T) {
	g := newGrowthTracker()
	id := inventory.CoreID(inventory.Kind("Volume"), "ns", "v")
	at := time.Unix(0, 0)
	for i := 0; i < minGrowthSamples-1; i++ {
		_, ok := g.observe(id, at.Add(time.Duration(i)*time.Minute),
			uint64(i*100), 10000)
		assert.False(t, ok)
	}
}

func TestGrowthTrackerEstimatesFillTime(t *testing.T) {
	g := newGrowthTracker()
	id := inventory.CoreID(inventory.Kind("Volume"), "ns", "v")
	at := time.Unix(0, 0)
	var eta time.Duration
	var ok bool
	for i := 0; i < minGrowthSamples; i++ {
		eta, ok = g.observe(id, at.Add(time.Duration(i)*time.Second),
			uint64(i*100), 10000)
	}
	assert.True(t, ok)
	// 100 bytes/s, 9100 left.
	assert.InDelta(t, 91, eta.Seconds(), 0.01)
}

func TestGrowthTrackerIgnoresFlatAndShrinkingUsage(t *testing.T) {
	g := newGrowthTracker()
	id := inventory.CoreID(inventory.Kind("Volume"), "ns", "v")
	at := time.Unix(0, 0)
	for i := 0; i < growthSamples+5; i++ {
		_, ok := g.observe(id, at.Add(time.Duration(i)*time.Second),
			5000, 10000)
		assert.False(t, ok)
	}
	assert.Len(t, g.samples[id], growthSamples)
	assert.Zero(t, bytesPerSecond([]usageSample{{at: at}, {at: at}}))
}

func TestGrowthTrackerForgetsUnseenVolumes(t *testing.T) {
	g := newGrowthTracker()
	old := inventory.CoreID(inventory.Kind("Volume"), "ns", "old")
	live := inventory.CoreID(inventory.Kind("Volume"), "ns", "live")
	at := time.Unix(0, 0)
	g.observe(old, at, 10, 100)
	g.observe(live, at.Add(time.Hour), 10, 100)

	g.forgetBefore(at.Add(30 * time.Minute))

	assert.NotContains(t, g.samples, old)
	assert.Contains(t, g.samples, live)
}

func TestGrowthTrackerIgnoresFillTimesBeyondTheBound(t *testing.T) {
	g := newGrowthTracker()
	id := inventory.CoreID(inventory.Kind("Volume"), "ns", "v")
	at := time.Unix(0, 0)
	var ok bool
	// One byte per minute on a 100 TB volume: thousands of years left.
	for i := 0; i < minGrowthSamples; i++ {
		_, ok = g.observe(id, at.Add(time.Duration(i)*time.Minute),
			uint64(i), 100_000_000_000_000)
	}
	assert.False(t, ok)
}
