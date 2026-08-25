package handler

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type oomEvent struct {
	Time time.Time
}

type oomTracker struct {
	mu        sync.Mutex
	records   map[string][]oomEvent
	threshold int
	window    time.Duration
	now       func() time.Time
}

func newOomTracker(threshold int, window time.Duration) *oomTracker {
	return &oomTracker{
		records:   make(map[string][]oomEvent),
		threshold: threshold,
		window:    window,
		now:       time.Now,
	}
}

const maxOomEntries = 100
const maxOomKeys = 500

// record adds a timestamp for the given key and returns the count of
// events within the sliding window, and whether the threshold is met.
func (t *oomTracker) record(key string) (count int, isRepeating bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	cutoff := now.Add(-t.window)

	entries := t.records[key]
	start := 0
	for start < len(entries) && entries[start].Time.Before(cutoff) {
		start++
	}
	entries = append(entries[start:], oomEvent{Time: now})
	if len(entries) > maxOomEntries {
		entries = entries[len(entries)-maxOomEntries:]
	}
	t.records[key] = entries

	if len(t.records) > maxOomKeys {
		t.pruneStaleLocked(cutoff)
	}

	count = len(entries)
	return count, count >= t.threshold
}

// pruneStaleLocked removes tracker keys whose newest event predates cutoff,
// preventing unbounded growth from short-lived pods. Caller holds t.mu.
func (t *oomTracker) pruneStaleLocked(cutoff time.Time) {
	for key, entries := range t.records {
		if len(entries) == 0 || entries[len(entries)-1].Time.Before(cutoff) {
			delete(t.records, key)
		}
	}
}

// History returns a formatted timeline of OOM events for the given key.
// Format: "OOM at 12:34:56, 12:38:01, 12:41:12"
func (t *oomTracker) History(key string) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries := t.records[key]
	if len(entries) == 0 {
		return ""
	}

	sorted := make([]oomEvent, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Time.Before(sorted[j].Time)
	})

	times := make([]string, 0, len(sorted))
	for _, e := range sorted {
		times = append(times, e.Time.Format("15:04:05"))
	}
	return "OOM at " + strings.Join(times, ", ")
}
