package issues

import (
	"strconv"
	"strings"
	"time"
)

// HasThread implements delivery.ThreadLookup: it reports whether key was
// ever given an issue, including one whose reference could not be read
// (the untracked marker). It copies nothing.
func (m *Map) HasThread(key string) bool {
	id, _ := m.lookup(key)
	return id != ""
}

// SnapshotThreads implements delivery.ThreadStateProvider. Untracked
// markers are not persisted: the stored format holds real issue ids only,
// so after a restart such an incident is unknown and may open a new issue.
// A closed issue is stored as "id", a separator and the Unix time its
// reopen window ends.
func (m *Map) SnapshotThreads() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	out := make(map[string]string, len(m.ids))
	for key, id := range m.ids {
		if id == untracked {
			continue
		}
		if until, closed := m.closedUntil[key]; closed {
			id += closedSeparator + strconv.FormatInt(until.Unix(), 10)
		}
		out[key] = id
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (m *Map) RestoreThreads(saved map[string]string) {
	for key, stored := range saved {
		id, until, closed := parseStored(stored)
		if key == "" || id == "" || id == untracked {
			continue
		}
		if existing, _ := m.lookup(key); existing != "" {
			continue
		}
		if closed && !m.now().Before(until) {
			continue
		}
		m.remember(key, id)
		if closed {
			m.mu.Lock()
			m.closedUntil[key] = until
			m.mu.Unlock()
		}
	}
}

// closedSeparator splits a stored closed issue into its id and the end of
// its reopen window. It cannot appear in an issue id.
const closedSeparator = "\x1f"

// parseStored reads a persisted value back: a bare id is an open issue.
func parseStored(stored string) (id string, until time.Time, closed bool) {
	id, rest, found := strings.Cut(stored, closedSeparator)
	if !found {
		return id, time.Time{}, false
	}
	seconds, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return id, time.Time{}, false
	}
	return id, time.Unix(seconds, 0), true
}
