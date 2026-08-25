package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFirstSeenMarkReturnsFirstTime(t *testing.T) {
	f := newFirstSeen()
	now := time.Now()
	first := f.mark("k", now)
	second := f.mark("k", now.Add(time.Minute))
	assert.Equal(t, first, second, "mark must return the first observed time")
}

func TestFirstSeenClear(t *testing.T) {
	f := newFirstSeen()
	now := time.Now()
	f.mark("k", now)
	f.clear("k")
	assert.Empty(t, f.dump())
	// After clear a fresh window starts.
	first := f.mark("k", now.Add(time.Hour))
	assert.Equal(t, now.Add(time.Hour), first)
}

func TestFirstSeenClearPrefix(t *testing.T) {
	f := newFirstSeen()
	now := time.Now()
	f.mark("node1/DiskPressure", now)
	f.mark("node1/PIDPressure", now)
	f.mark("node2/MemoryPressure", now)
	f.clearPrefix("node1/")
	assert.Len(t, f.dump(), 1)
	assert.Contains(t, f.dump(), "node2/MemoryPressure")
}

func TestFirstSeenPruneEvictsStaleKeys(t *testing.T) {
	f := newFirstSeen()
	f.maxAge = time.Minute
	now := time.Now()
	f.seed("stale", now.Add(-2*time.Minute))
	f.seed("fresh", now.Add(-30*time.Second))
	f.pruneLocked(now)
	assert.NotContains(t, f.dump(), "stale")
	assert.Contains(t, f.dump(), "fresh")
}

func TestFirstSeenMarkKeepsActiveKeyAlive(t *testing.T) {
	f := newFirstSeen()
	f.maxAge = time.Hour
	now := time.Now()
	// Key first failed long ago but is still emitting events.
	f.seed("active", now.Add(-2*time.Hour))
	f.mark("active", now.Add(-time.Minute))
	f.pruneLocked(now)
	assert.Contains(t, f.dump(), "active", "actively failing key must survive pruning")
}
