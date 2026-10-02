package issues

import (
	"context"
	"sync"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

// maxTrackedIssues bounds the problem-to-issue map.
const maxTrackedIssues = 1000

// untracked marks an incident whose issue was created but whose reference
// could not be read from the tracker response. It keeps later updates
// from opening a duplicate issue. The NUL byte keeps it apart from any
// real issue id.
const untracked = "\x00untracked"

// Tracker is the provider-specific issue API.
type Tracker interface {
	Name() string
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

// Deliver files, updates or closes the issue for one incident message.
// The first message of a key creates the issue, later ones comment on it,
// and the resolve message comments and closes it. Plain notices such as
// the startup banner, and the startup summary, are not incidents and are
// skipped: each problem the summary lists gets its own issue.
func (m *Map) Deliver(
	ctx context.Context,
	tracker Tracker,
	msg notification.Message,
	title, body string,
) error {
	if msg.IsInformational() {
		return nil
	}
	key := msg.ThreadKey()
	id := m.lookup(key)
	switch {
	case msg.Resolved() && id == "":
		return nil
	case id == untracked:
		return m.skipUntracked(tracker, msg, key)
	case msg.Resolved():
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
	if id == "" {
		id = untracked
		klog.InfoS("issue created without a readable reference; "+
			"later updates for this incident are skipped",
			"component", "delivery", "operation", "create_issue",
			"provider", tracker.Name(), "key", key)
		metrics.DefaultRegistry().Delivery.TrackerUntracked.Add(1)
	}
	m.remember(key, id)
	return nil
}

// skipUntracked drops an update for an incident whose issue cannot be
// addressed. A resolve ends the conversation and clears the marker, so a
// later recurrence opens a fresh issue.
func (m *Map) skipUntracked(
	tracker Tracker, msg notification.Message, key string,
) error {
	if msg.Resolved() {
		m.forget(key)
	}
	klog.V(1).InfoS("skipping update for an untracked issue",
		"component", "delivery", "operation", "update_issue",
		"provider", tracker.Name(), "key", key,
		"resolved", msg.Resolved())
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

// SnapshotThreads implements delivery.ThreadStateProvider. Untracked
// markers are not persisted: the stored format holds real issue ids only,
// so after a restart such an incident is unknown and may open a new issue.
func (m *Map) SnapshotThreads() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]string, len(m.ids))
	for key, id := range m.ids {
		if id != untracked {
			out[key] = id
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (m *Map) RestoreThreads(saved map[string]string) {
	for key, id := range saved {
		if key != "" && id != "" && id != untracked &&
			m.lookup(key) == "" {
			m.remember(key, id)
		}
	}
}
