package pod

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
)

type oomEvent struct {
	Time time.Time
}

type oomTracker struct {
	mu          sync.Mutex
	records     map[string][]oomEvent
	threshold   int
	window      time.Duration
	clockSource clock.Clock
}

func newOOMTracker(
	threshold int,
	window time.Duration,
	runtimeClock clock.Clock,
) *oomTracker {
	if runtimeClock == nil {
		runtimeClock = clock.RealClock{}
	}
	return &oomTracker{
		records:     make(map[string][]oomEvent),
		threshold:   threshold,
		window:      window,
		clockSource: runtimeClock,
	}
}

const maxOOMEntries = 100
const maxOOMKeys = 500

func (t *oomTracker) record(key string) (count int, repeating bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.clockSource.Now()
	cutoff := now.Add(-t.window)
	entries := t.records[key]
	start := 0
	for start < len(entries) && entries[start].Time.Before(cutoff) {
		start++
	}
	entries = append(entries[start:], oomEvent{Time: now})
	if len(entries) > maxOOMEntries {
		entries = entries[len(entries)-maxOOMEntries:]
	}
	t.records[key] = entries
	if len(t.records) > maxOOMKeys {
		t.pruneStaleLocked(cutoff)
	}

	count = len(entries)
	return count, count >= t.threshold
}

func (t *oomTracker) pruneStaleLocked(cutoff time.Time) {
	for key, entries := range t.records {
		if len(entries) == 0 || entries[len(entries)-1].Time.Before(cutoff) {
			delete(t.records, key)
		}
	}
}

func (t *oomTracker) history(key string) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries := t.records[key]
	if len(entries) == 0 {
		return ""
	}
	sortedEntries := make([]oomEvent, len(entries))
	copy(sortedEntries, entries)
	sort.Slice(sortedEntries, func(i, j int) bool {
		return sortedEntries[i].Time.Before(sortedEntries[j].Time)
	})

	times := make([]string, 0, len(sortedEntries))
	for _, entry := range sortedEntries {
		times = append(times, entry.Time.Format("15:04:05"))
	}
	return "OOM at " + strings.Join(times, ", ")
}
