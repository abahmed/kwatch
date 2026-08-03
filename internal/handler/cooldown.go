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
type firstSeen struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func newFirstSeen() *firstSeen {
	return &firstSeen{m: make(map[string]time.Time)}
}

// mark returns the first observed time for key, recording now on first use.
// Callers pass their own clock value so time remains injectable in tests.
func (f *firstSeen) mark(key string, now time.Time) time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.m[key]; ok {
		return t
	}
	f.m[key] = now
	return now
}

// clear forgets key so the next observation starts a fresh window.
func (f *firstSeen) clear(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, key)
}

// clearPrefix forgets every key with the given prefix.
func (f *firstSeen) clearPrefix(prefix string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.m {
		if strings.HasPrefix(k, prefix) {
			delete(f.m, k)
		}
	}
}

// get returns the recorded time for key, if any.
func (f *firstSeen) get(key string) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.m[key]
	return t, ok
}

// seed records a specific time for key, used by tests to simulate history.
func (f *firstSeen) seed(key string, t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[key] = t
}

// dump returns a copy of the recorded entries, used by tests to inspect state.
func (f *firstSeen) dump() map[string]time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]time.Time, len(f.m))
	for k, v := range f.m {
		out[k] = v
	}
	return out
}
