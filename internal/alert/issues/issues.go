package issues

import (
	"context"
	"sync"

	"github.com/abahmed/kwatch/internal/event"
)

// maxTrackedIssues bounds the problem-to-issue map.
const maxTrackedIssues = 1000

// Tracker is the provider-specific issue API.
type Tracker interface {
	Create(ctx context.Context, title, body string) (string, error)
	Comment(ctx context.Context, id, body string) error
	// Close ends the issue; trackers without a generic close just comment.
	Close(ctx context.Context, id, body string) error
}

// Map remembers which issue speaks for which problem. It is persisted
// through the delivery thread store so a restart keeps commenting on the
// same issue instead of opening another.
type Map struct {
	mu    sync.Mutex
	ids   map[string]string
	order []string
}

// NewMap returns an empty issue map.
func NewMap() *Map {
	return &Map{ids: make(map[string]string)}
}

// Deliver files, updates or closes the issue for one event. Notices such as
// the startup banner are not issues and are skipped.
func (m *Map) Deliver(
	ctx context.Context,
	tracker Tracker,
	e *event.Event,
	title, body string,
) error {
	if e.IsNotice() {
		return nil
	}
	key := e.AlertKey()
	id := m.lookup(key)
	switch {
	case e.IsResolve() && id == "":
		return nil
	case e.IsResolve():
		if err := tracker.Close(ctx, id, body); err != nil {
			return err
		}
		m.forget(key)
		return nil
	case id != "":
		return tracker.Comment(ctx, id, body)
	}
	id, err := tracker.Create(ctx, title, body)
	if err != nil {
		return err
	}
	if e.DedupKey != "" && id != "" {
		m.remember(key, id)
	}
	return nil
}

func (m *Map) lookup(key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ids[key]
}

func (m *Map) remember(key, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.ids[key]; !exists {
		m.order = append(m.order, key)
	}
	m.ids[key] = id
	for len(m.ids) > maxTrackedIssues && len(m.order) > 0 {
		delete(m.ids, m.order[0])
		m.order = m.order[1:]
	}
}

func (m *Map) forget(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.ids, key)
	for i, candidate := range m.order {
		if candidate == key {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

// SnapshotThreads implements delivery.ThreadStateProvider.
func (m *Map) SnapshotThreads() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.ids) == 0 {
		return nil
	}
	out := make(map[string]string, len(m.ids))
	for key, id := range m.ids {
		out[key] = id
	}
	return out
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (m *Map) RestoreThreads(saved map[string]string) {
	for key, id := range saved {
		if key != "" && id != "" && m.lookup(key) == "" {
			m.remember(key, id)
		}
	}
}
