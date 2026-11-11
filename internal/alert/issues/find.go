package issues

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

// ForCluster sets the cluster name that is part of the marker new issues
// carry, so two clusters filing into one project never adopt or close
// each other's issues. It is called once, at construction.
func (m *Map) ForCluster(name string) *Map {
	m.cluster = name
	return m
}

// markers are the search tokens of msg's issue, the one a new issue
// carries first. That one is made of the alert key, which is derived
// from what fails and so survives a reset of kwatch's state. The second
// is made of the conversation key, which does not: it finds the issues
// filed before the alert key was used. A message without an alert key
// has only the second.
func (m *Map) markers(msg notification.Message, key string) []string {
	if msg.DedupKey == "" {
		return []string{Marker(key)}
	}
	return []string{Marker(msg.AlertKey(m.cluster)), Marker(key)}
}

// create files the issue for key. A tracker that can search for an issue
// by marker is asked first: a create whose answer was lost (timeout) may
// have succeeded, and creating again would duplicate the issue.
//
// An issue found for a later message of the incident (the mapping was
// lost: state saved late, evicted, or reset) gets that message as a
// comment, or its text would be lost. An issue found after a create
// that failed here is that create and already carries the text.
func (m *Map) create(
	ctx context.Context, tracker Tracker, msg notification.Message,
	key, title, body string,
) error {
	markers := m.markers(msg, key)
	existing, searched, err := findOpen(ctx, tracker, markers)
	if err != nil {
		return err
	}
	if existing != "" {
		ownCreate := m.triedCreate(key)
		m.remember(key, existing)
		if msg.Revision > 1 && !ownCreate {
			return tracker.Comment(ctx, existing, body)
		}
		return nil
	}
	if searched {
		body += "\n\nRef: " + markers[0]
	}
	id, err := tracker.Create(ctx, title, body)
	if err != nil {
		m.markTriedCreate(key)
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

// closeUnmapped handles a resolve for a key that has no issue mapped.
// The issue may still exist: the mapping is saved with a delay, is
// bounded, and is lost with kwatch's state. A tracker that can search is
// asked for it, and the issue it finds is closed. Nothing is mapped
// afterwards: the incident is over.
func (m *Map) closeUnmapped(
	ctx context.Context, tracker Tracker, msg notification.Message,
	key, body string,
) error {
	id, _, err := findOpen(ctx, tracker, m.markers(msg, key))
	if err != nil || id == "" {
		return err
	}
	err = m.closeIssue(ctx, tracker, key, id, body)
	if err != nil && !transport.IsNotFound(err) {
		return err
	}
	m.clearResolveCommented(key)
	return nil
}

// findOpen asks the tracker for an open issue carrying one of markers,
// in order. searched is false when the tracker cannot search. A search
// that fails for a reason a retry may fix is returned, so the create is
// not repeated blindly; a search the tracker refuses for good (no
// permission, unsupported) is logged and nothing is found.
func findOpen(
	ctx context.Context, tracker Tracker, markers []string,
) (id string, searched bool, err error) {
	finder, ok := tracker.(Finder)
	if !ok {
		return "", false, nil
	}
	for _, marker := range markers {
		id, err := finder.Find(ctx, marker)
		switch {
		case err == nil && id != "":
			return id, true, nil
		case err == nil:
			continue
		case !transport.IsPermanent(err):
			return "", true, err
		}
		klog.InfoS("issue search refused; continuing without it",
			"component", "delivery", "operation", "find_issue",
			"provider", tracker.Name())
		return "", true, nil
	}
	return "", true, nil
}

func (m *Map) triedCreate(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, tried := m.tried[key]
	return tried
}

// markTriedCreate notes a failed create. The set is bounded like the
// issue map; when it is full it starts over, which at worst repeats one
// message as a comment.
func (m *Map) markTriedCreate(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tried) >= maxTrackedIssues {
		m.tried = make(map[string]struct{})
	}
	m.tried[key] = struct{}{}
}
