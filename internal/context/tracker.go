package context

import (
	"sync"
	"time"
)

type ChangeType int

const (
	ChangeCreate ChangeType = iota
	ChangeUpdate
	ChangeDelete
)

func (t ChangeType) String() string {
	switch t {
	case ChangeCreate:
		return "created"
	case ChangeUpdate:
		return "updated"
	case ChangeDelete:
		return "deleted"
	default:
		return "unknown"
	}
}

type Change struct {
	Resource  string
	Namespace string
	Name      string
	Type      ChangeType
	Timestamp time.Time
	Detail    string
}

type ChangeTracker struct {
	mu     sync.RWMutex
	buffer []Change
	head   int
	count  int
}

// Snapshot returns changes from oldest to newest so the history can be safely
// persisted and restored without exposing the ring-buffer internals.
func (t *ChangeTracker) Snapshot() []Change {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Change, 0, t.count)
	start := (t.head - t.count + len(t.buffer)) % len(t.buffer)
	for i := 0; i < t.count; i++ {
		out = append(out, t.buffer[(start+i)%len(t.buffer)])
	}
	return out
}

// Restore replaces the in-memory history with a previously persisted window.
// Invalid/old entries are harmless; the ring retains only its configured size.
func (t *ChangeTracker) Restore(changes []Change) {
	t.mu.Lock()
	t.buffer = make([]Change, len(t.buffer))
	t.head, t.count = 0, 0
	t.mu.Unlock()
	for _, change := range changes {
		t.Record(change)
	}
}

const defaultTrackedChanges = 1000

func NewChangeTracker(capacity int) *ChangeTracker {
	if capacity <= 0 {
		capacity = defaultTrackedChanges
	}
	return &ChangeTracker{
		buffer: make([]Change, capacity),
	}
}

func (t *ChangeTracker) Record(c Change) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buffer[t.head] = c
	t.head = (t.head + 1) % len(t.buffer)
	if t.count < len(t.buffer) {
		t.count++
	}
}

func (t *ChangeTracker) RecentChangesBefore(age time.Duration) []Change {
	return t.RecentChangesBeforeAt(age, time.Now())
}

func (t *ChangeTracker) RecentChangesBeforeAt(age time.Duration, now time.Time) []Change {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cutoff := now.Add(-age)
	var out []Change
	for i := 0; i < t.count; i++ {
		idx := (t.head - 1 - i + len(t.buffer)) % len(t.buffer)
		if t.buffer[idx].Timestamp.After(cutoff) {
			out = append(out, t.buffer[idx])
		}
	}
	return out
}
