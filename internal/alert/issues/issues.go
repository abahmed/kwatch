package issues

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/transport"
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

// Finder is optional. A tracker that implements it can find an open issue
// whose body carries marker, which makes creating an issue idempotent: a
// create retried after a lost answer finds the issue it already made.
type Finder interface {
	// Find returns the id of an open issue carrying marker, or "".
	Find(ctx context.Context, marker string) (string, error)
}

// Marker is the search token that ties an issue to its incident key. It is
// one alphanumeric word, so every tracker's text search treats it alike.
func Marker(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "kwatchkey" + hex.EncodeToString(sum[:8])
}

// Reopener is optional. A tracker that implements it reopens a closed
// issue when its incident fails again within the reopen window.
//
// A tracker that cannot reopen would comment on a closed issue nobody
// watches, so for it a resolve forgets the mapping and a recurrence opens
// a new issue. A tracker that did not close the issue at all (no close
// setting) keeps the mapping: the issue is still open. A Reopener that is
// only configured sometimes can also implement CanReopen to say so.
type Reopener interface {
	Reopen(ctx context.Context, id string) error
}

// canReopen reports whether tracker can reopen the issues it closes.
func canReopen(tracker Tracker) bool {
	r, ok := tracker.(Reopener)
	if !ok {
		return false
	}
	if c, ok := r.(interface{ CanReopen() bool }); ok {
		return c.CanReopen()
	}
	return true
}

// closesIssues reports whether tracker's Close really ends the issue. A
// tracker that can be configured not to (no close transition or status)
// implements ClosesIssues to say so; any other tracker is taken to close.
func closesIssues(tracker Tracker) bool {
	if c, ok := tracker.(interface{ ClosesIssues() bool }); ok {
		return c.ClosesIssues()
	}
	return true
}

// Map remembers which issue speaks for which problem. It is persisted
// through the delivery thread store so a restart keeps commenting on the
// same issue instead of opening another.
//
// A resolve that may reopen (Message.ReopenWithin) closes the issue but
// keeps the mapping until the window ends, like Slack keeps a thread: a
// failure inside the window reopens and comments on the same issue. A
// tracker that cannot reopen gets no window.
type Map struct {
	mu    sync.Mutex
	ids   map[string]string
	order []string
	// closedUntil holds the keys whose issue is closed but still mapped,
	// with the end of their reopen window.
	closedUntil map[string]time.Time
	now         func() time.Time
}

// NewMap returns an empty issue map that reads time from now.
func NewMap(now func() time.Time) *Map {
	return &Map{
		ids:         make(map[string]string),
		closedUntil: make(map[string]time.Time),
		now:         clock.RequireFunc(now),
	}
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
	id, closed := m.lookup(key)
	switch {
	case msg.Resolved() && id == "":
		return nil
	case id == untracked:
		return m.skipUntracked(tracker, msg, key)
	case msg.Resolved() && closed:
		// Already closed and waiting out its reopen window.
		return nil
	case msg.Resolved():
		if err := tracker.Close(ctx, id, body); err != nil &&
			!transport.IsNotFound(err) {
			return err
		}
		if !closesIssues(tracker) {
			// Only a comment was added: the issue is still open, so the
			// mapping stays and a recurrence comments on it.
			return nil
		}
		// A closed issue, or one already deleted (404), is finished: the
		// mapping must go, or every retry would fail the same way.
		window := msg.ReopenWithin
		if !canReopen(tracker) {
			window = 0
		}
		m.finish(key, window)
		return nil
	case id != "":
		err := m.update(ctx, tracker, key, id, closed, body)
		if !transport.IsNotFound(err) {
			return err
		}
		// The issue was deleted: forget it and open a new one below.
		m.forget(key)
	}
	return m.create(ctx, tracker, key, title, body)
}

// create files the issue for key. A tracker that can search for an issue
// by marker is asked first: a create whose answer was lost (timeout) may
// have succeeded, and creating again would duplicate the issue.
func (m *Map) create(
	ctx context.Context, tracker Tracker, key, title, body string,
) error {
	finder, searchable := tracker.(Finder)
	if searchable {
		marker := Marker(key)
		existing, err := find(ctx, finder, tracker, marker)
		if err != nil {
			return err
		}
		if existing != "" {
			m.remember(key, existing)
			return nil
		}
		body += "\n\nRef: " + marker
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

// find asks the tracker for an open issue carrying marker. A search that
// fails for a reason a retry may fix is returned, so the create is not
// repeated blindly; a search the tracker refuses for good (no permission,
// unsupported) is logged and the issue is created as before.
func find(
	ctx context.Context, finder Finder, tracker Tracker, marker string,
) (string, error) {
	id, err := finder.Find(ctx, marker)
	if err == nil {
		return id, nil
	}
	if !transport.IsPermanent(err) {
		return "", err
	}
	klog.InfoS("issue search refused; creating without it",
		"component", "delivery", "operation", "find_issue",
		"provider", tracker.Name())
	return "", nil
}

// update comments on the issue of a live incident. When the issue is
// closed (the incident is failing again inside its reopen window) the
// tracker reopens it first, if it can, and the mapping is open again.
func (m *Map) update(
	ctx context.Context, tracker Tracker, key, id string,
	closed bool, body string,
) error {
	if closed {
		if r, ok := tracker.(Reopener); ok {
			if err := r.Reopen(ctx, id); err != nil {
				return err
			}
		}
	}
	if err := tracker.Comment(ctx, id, body); err != nil {
		return err
	}
	if closed {
		m.reopened(key)
	}
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

// lookup returns the issue id for key and whether that issue is closed.
// A closed issue whose reopen window has ended is forgotten first.
func (m *Map) lookup(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	_, closed := m.closedUntil[key]
	return m.ids[key], closed
}

// finish ends a resolved incident: with a reopen window the issue stays
// mapped (closed) until the window ends; otherwise the mapping goes.
func (m *Map) finish(key string, window time.Duration) {
	if window <= 0 {
		m.forget(key)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closedUntil[key] = m.now().Add(window)
}

// reopened marks key's issue open again.
func (m *Map) reopened(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.closedUntil, key)
}

// expireLocked forgets closed issues whose reopen window has ended.
func (m *Map) expireLocked() {
	now := m.now()
	for key, until := range m.closedUntil {
		if !now.Before(until) {
			m.forgetLocked(key)
		}
	}
}

func (m *Map) remember(key, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	delete(m.closedUntil, key)
	if _, exists := m.ids[key]; !exists {
		m.order = append(m.order, key)
	}
	m.ids[key] = id
	m.evictLocked()
}

// evictLocked keeps the map at maxTrackedIssues. Closed issues go first
// (oldest first): their incident is over and only a reopen window is lost.
// An open issue is dropped only when nothing closed is left.
func (m *Map) evictLocked() {
	for len(m.ids) > maxTrackedIssues && len(m.order) > 0 {
		victim := 0
		for i, candidate := range m.order {
			if _, closed := m.closedUntil[candidate]; closed {
				victim = i
				break
			}
		}
		delete(m.ids, m.order[victim])
		delete(m.closedUntil, m.order[victim])
		m.order = append(m.order[:victim], m.order[victim+1:]...)
	}
}

func (m *Map) forget(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forgetLocked(key)
}

func (m *Map) forgetLocked(key string) {
	delete(m.ids, key)
	delete(m.closedUntil, key)
	for i, candidate := range m.order {
		if candidate == key {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}
