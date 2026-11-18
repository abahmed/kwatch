package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// The served revisions of a workload are the model's memory of which
// of its revisions kwatch saw serving (inventory.ServedRevision), so a
// rollback after a completed rollout can still be backed by evidence.
// They are kept beside the workload's last-healthy mark in its
// healthyRecord, restored at startup, checked against the cluster once
// listed (served_restore.go) and forgotten when the workload is
// deleted.

// served remembers the revision a workload serves when an observation
// shows its current revision with something ready, and forgets the
// memory of a deleted workload, both in the model and on disk.
func (h *historyRecorder) served(o inventory.Observation) {
	switch o.Kind {
	case inventory.Gone, inventory.Changed:
		if !incident.IsWorkload(o.Entity.Kind) ||
			(o.Kind == inventory.Changed && !o.Change.Deleted) {
			return
		}
		h.dropServed(o.Entity)
	case inventory.Observed, inventory.Related:
		workload, name, ok := explain.ServingRevision(h.model, o.Entity)
		at := o.At
		if at.IsZero() {
			at = h.now()
		}
		if !ok || !h.model.MarkServed(workload, name, at) {
			return
		}
		if h.writer != nil {
			h.writer.addServed(workload,
				h.model.ServedRevisions(workload, time.Time{}))
		}
	}
}

// restoreServed puts a saved record's served revisions back into the
// model, leaving out those last seen more than
// inventory.ServedRetention before now, and marks the workload for the
// check once its kind is fully listed (served_restore.go).
func (h *historyRecorder) restoreServed(
	id inventory.EntityID, record healthyRecord, now time.Time,
) {
	var since time.Time
	if !now.IsZero() {
		since = now.Add(-inventory.ServedRetention)
	}
	h.model.RestoreServed(id, record.Served, since)
	if len(h.model.ServedRevisions(id, time.Time{})) > 0 {
		h.restored.add(id)
	}
}

// addServed queues a workload's served revisions for its healthy
// record.
func (w *historyWriter) addServed(
	id inventory.EntityID, served []inventory.ServedRevision,
) {
	w.queueHealthy(id, healthyRecord{Served: served})
}

// forgetServed queues dropping a deleted workload's served revisions.
func (w *historyWriter) forgetServed(id inventory.EntityID) {
	w.queueHealthy(id, healthyRecord{forget: true})
}

// queueHealthy merges a partial record into the one waiting for the
// entity, the newer fields winning.
func (w *historyWriter) queueHealthy(
	id inventory.EntityID, record healthyRecord,
) {
	w.mu.Lock()
	if w.healthy == nil {
		w.healthy = map[string]healthyRecord{}
	}
	key := id.String()
	w.healthy[key] = mergeHealthy(w.healthy[key], record)
	w.mu.Unlock()
	notify(w.wake)
}

// mergeHealthy fills what newer leaves unset from older: a zero mark
// keeps the older mark, and nil Served without forget keeps the older
// served revisions (or the older forget).
func mergeHealthy(older, newer healthyRecord) healthyRecord {
	out := newer
	if out.Mark.At.IsZero() {
		out.Mark = older.Mark
	}
	if out.Served == nil && !out.forget {
		out.Served, out.forget = older.Served, older.forget
	}
	return out
}

// partial reports a record that leaves its mark or its served
// revisions unset, so a save must keep the stored ones.
func (r healthyRecord) partial() bool {
	return r.Mark.At.IsZero() || (r.Served == nil && !r.forget)
}

// latest is the newest time a record speaks of: its mark or its
// newest served revision.
func (r healthyRecord) latest() time.Time {
	at := r.Mark.At
	for _, s := range r.Served {
		if s.At.After(at) {
			at = s.At
		}
	}
	return at
}

// expires is when a stored record lapses: healthyRetention after its
// mark or inventory.ServedRetention after its newest served revision,
// whichever is later.
func (r healthyRecord) expires() time.Time {
	at := r.Mark.At.Add(healthyRetention)
	for _, s := range r.Served {
		if end := s.At.Add(inventory.ServedRetention); end.After(at) {
			at = end
		}
	}
	return at
}
