package inventory

import (
	"slices"
	"time"
)

// A workload's served revisions are kwatch's memory of which of its
// revisions it saw serving: the revision was current and had a ready
// replica or pod. A completed rollout scales the old revision to zero
// and its pods go, so without this memory nothing would show that the
// revision a rollback returns to ever ran.
const (
	// MaxServedRevisions bounds the revisions remembered per workload:
	// the most recently served are kept.
	MaxServedRevisions = 5
	// ServedRetention is how long a revision stays remembered after it
	// was last seen serving.
	ServedRetention = 30 * 24 * time.Hour
	// servedRefresh is how stale the last-seen time of a remembered
	// revision may grow before a new sighting moves it, so a busy
	// workload does not rewrite its memory on every status update.
	servedRefresh = time.Hour
)

// ServedRevision is one revision of a workload kwatch saw serving.
type ServedRevision struct {
	// Name is the object that holds the revision: the ReplicaSet of a
	// Deployment or the ControllerRevision of a StatefulSet or
	// DaemonSet. The name stays the same when a rollback renumbers the
	// revision.
	Name string
	// At is when kwatch last saw it serving.
	At time.Time
}

// MarkServed records that workload's revision name was seen serving at
// at. It reports whether the memory changed: a new revision, or one
// whose last sighting is more than an hour old. Revisions last seen
// more than ServedRetention before at are forgotten, and only the
// MaxServedRevisions most recent are kept.
func (m *Model) MarkServed(workload EntityID, name string, at time.Time) bool {
	if workload.IsZero() || name == "" || at.IsZero() {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.recordFor(workload)
	i := slices.IndexFunc(rec.served, func(s ServedRevision) bool {
		return s.Name == name
	})
	if i >= 0 && at.Sub(rec.served[i].At) < servedRefresh {
		return false
	}
	if i >= 0 {
		rec.served[i].At = at
	} else {
		rec.served = append(rec.served, ServedRevision{Name: name, At: at})
	}
	rec.served = boundServed(rec.served, at.Add(-ServedRetention))
	return true
}

// RestoreServed puts a workload's served revisions saved by an earlier
// run back. Revisions last seen before since are left out; the bound
// applies as for MarkServed.
func (m *Model) RestoreServed(
	workload EntityID, served []ServedRevision, since time.Time,
) {
	if workload.IsZero() || len(served) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.recordFor(workload)
	for _, s := range served {
		if s.Name == "" || s.At.IsZero() ||
			slices.ContainsFunc(rec.served, func(r ServedRevision) bool {
				return r.Name == s.Name
			}) {
			continue
		}
		rec.served = append(rec.served, s)
	}
	rec.served = boundServed(rec.served, since)
}

// ForgetServed drops workload's served revisions, as when the workload
// is deleted: a workload created again under its name starts with no
// record of having served.
func (m *Model) ForgetServed(workload EntityID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rec := m.records[workload]; rec != nil {
		rec.served = nil
	}
}

// ForgetServedBefore drops workload's served revisions last seen
// before t, the time the workload was created: a revision seen serving
// before then served an earlier object of the same name, deleted while
// kwatch was not watching. It returns the revisions left and whether
// any was dropped.
func (m *Model) ForgetServedBefore(
	workload EntityID, t time.Time,
) ([]ServedRevision, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec := m.records[workload]
	if rec == nil || t.IsZero() {
		return nil, false
	}
	kept := slices.DeleteFunc(slices.Clone(rec.served),
		func(s ServedRevision) bool { return s.At.Before(t) })
	if len(kept) == len(rec.served) {
		return kept, false
	}
	if len(kept) == 0 {
		kept = nil
	}
	rec.served = kept
	return slices.Clone(kept), true
}

// ServedRevisions returns the revisions of workload last seen serving
// at or after since, oldest sighting first.
func (m *Model) ServedRevisions(
	workload EntityID, since time.Time,
) []ServedRevision {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec := m.records[workload]
	if rec == nil {
		return nil
	}
	var out []ServedRevision
	for _, s := range rec.served {
		if !s.At.Before(since) {
			out = append(out, s)
		}
	}
	return out
}

// boundServed drops revisions last seen before since, orders the rest
// by sighting and keeps the MaxServedRevisions most recent.
func boundServed(served []ServedRevision, since time.Time) []ServedRevision {
	kept := served[:0]
	for _, s := range served {
		if !s.At.Before(since) {
			kept = append(kept, s)
		}
	}
	slices.SortStableFunc(kept, func(a, b ServedRevision) int {
		return a.At.Compare(b.At)
	})
	if over := len(kept) - MaxServedRevisions; over > 0 {
		kept = append(kept[:0:0], kept[over:]...)
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}
