package issues

import (
	"context"
)

// StateCloser is optional. A tracker that splits its close into a comment
// and a state change implements it, so a retry after a failed state change
// does not post the resolve comment twice.
type StateCloser interface {
	// CloseState changes the issue state only; it posts no comment.
	CloseState(ctx context.Context, id string) error
}

// closeIssue ends the issue. The resolve comment is posted once per
// resolve: when the state change fails and the resolve is retried, only
// the state change is repeated.
func (m *Map) closeIssue(
	ctx context.Context, tracker Tracker, key, id, body string,
) error {
	closer, split := tracker.(StateCloser)
	if !split {
		return tracker.Close(ctx, id, body)
	}
	if !m.resolveCommented(key) {
		if err := tracker.Comment(ctx, id, body); err != nil {
			return err
		}
		m.markResolveCommented(key)
	}
	return closer.CloseState(ctx, id)
}

func (m *Map) resolveCommented(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, done := m.commented[key]
	return done
}

func (m *Map) markResolveCommented(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commented[key] = struct{}{}
}

// clearResolveCommented forgets the posted comment once the resolve is
// over, so the next resolve of the key comments again.
func (m *Map) clearResolveCommented(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.commented, key)
}
