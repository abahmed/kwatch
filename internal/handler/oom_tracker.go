package handler

import (
	"sync"
	"time"
)

type oomTracker struct {
	mu        sync.Mutex
	records   map[string][]time.Time
	threshold int
	window    time.Duration
	now       func() time.Time
}

func newOomTracker(threshold int, window time.Duration) *oomTracker {
	return &oomTracker{
		records:   make(map[string][]time.Time),
		threshold: threshold,
		window:    window,
		now:       time.Now,
	}
}

// record adds a timestamp for the given key and returns the count of
// events within the sliding window, and whether the threshold is met.
func (t *oomTracker) record(key string) (count int, isRepeating bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	cutoff := now.Add(-t.window)

	entries := t.records[key]
	start := 0
	for start < len(entries) && entries[start].Before(cutoff) {
		start++
	}
	entries = append(entries[start:], now)
	t.records[key] = entries

	count = len(entries)
	return count, count >= t.threshold
}
