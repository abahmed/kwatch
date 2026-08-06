package handler

import (
	"strings"
	"sync"
	"time"
)

// firstSeen records the first time a resource key was observed failing so a
// handler can hold off re-alerting until its hold-down window elapses. Each
// instance owns its own lock; create with newFirstSeen. It replaces the
// repeated mutex + map[string]time.Time pairs that were duplicated once per
// resource kind.
//
// Entries are bounded: a cap plus a staleness prune mirror the oomTracker
// strategy so keys for deleted or forgotten resources cannot grow without
// limit. Pruning only evicts keys that have stopped emitting failing events
// for maxAge, so an actively-failing resource never loses its window.
type firstSeen struct {
	mu       sync.Mutex
	first    map[string]time.Time // key → first observed failure time
	lastMark map[string]time.Time // key → most recent mark time
	maxAge   time.Duration
}

const firstSeenMaxAge = 1 * time.Hour
const firstSeenMaxEntries = 5000

func newFirstSeen() *firstSeen {
	return &firstSeen{
		first:    make(map[string]time.Time),
		lastMark: make(map[string]time.Time),
		maxAge:   firstSeenMaxAge,
	}
}

// mark returns the first observed time for key, recording now on first use.
// Callers pass their own clock value so time remains injectable in tests.
func (f *firstSeen) mark(key string, now time.Time) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastMark[key] = now
	if t, ok := f.first[key]; ok {
		return t
	}
	f.first[key] = now
	if len(f.first) > firstSeenMaxEntries {
		f.pruneLocked(now)
	}
	return now
}

// clear forgets key so the next observation starts a fresh window.
func (f *firstSeen) clear(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.first, key)
	delete(f.lastMark, key)
}

// clearPrefix forgets every key with the given prefix.
func (f *firstSeen) clearPrefix(prefix string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.first {
		if strings.HasPrefix(k, prefix) {
			delete(f.first, k)
		}
	}
	for k := range f.lastMark {
		if strings.HasPrefix(k, prefix) {
			delete(f.lastMark, k)
		}
	}
}

// get returns the recorded time for key, if any.
func (f *firstSeen) get(key string) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.first[key]
	return t, ok
}

// seed records a specific time for key, used by tests to simulate history.
func (f *firstSeen) seed(key string, t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.first[key] = t
	f.lastMark[key] = t
}

// dump returns a copy of the recorded entries, used by tests to inspect state.
func (f *firstSeen) dump() map[string]time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]time.Time, len(f.first))
	for k, v := range f.first {
		out[k] = v
	}
	return out
}

// pruneLocked evicts keys whose most recent mark predates maxAge. This bounds
// growth from resources that were deleted or never recovered without touching
// keys that are still failing. Caller holds f.mu.
func (f *firstSeen) pruneLocked(now time.Time) {
	if f.maxAge <= 0 {
		return
	}
	cutoff := now.Add(-f.maxAge)
	for k, last := range f.lastMark {
		if last.Before(cutoff) {
			delete(f.first, k)
			delete(f.lastMark, k)
		}
	}
}
