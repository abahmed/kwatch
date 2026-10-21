package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// addChange queues a change for the changes bucket when it is one a
// restart should remember. It never blocks.
func (w *historyWriter) addChange(change inventory.Change) {
	if !persistsChange(change) {
		return
	}
	w.mu.Lock()
	w.changes = append(w.changes, change)
	w.trimChangesLocked()
	w.mu.Unlock()
	notify(w.wake)
}

func (w *historyWriter) trimChangesLocked() {
	if over := len(w.changes) - maxPendingChanges; over > 0 {
		w.changes = append(w.changes[:0:0], w.changes[over:]...)
	}
}

// addHealthy queues the last healthy mark of a workload. A newer mark
// of the same entity replaces the queued one.
func (w *historyWriter) addHealthy(
	id inventory.EntityID, mark inventory.HealthMark,
) {
	w.mu.Lock()
	if w.healthy == nil {
		w.healthy = map[string]healthyRecord{}
	}
	w.healthy[id.String()] = healthyRecord{Mark: mark}
	w.mu.Unlock()
	notify(w.wake)
}

// flushChanges writes the queued changes with the change set each
// belongs to. A failed write is kept for the next batch. It reports
// false on failure.
func (w *historyWriter) flushChanges() bool {
	store, ok := w.store.(changeStore)
	w.mu.Lock()
	batch := w.changes
	w.changes = nil
	w.mu.Unlock()
	if !ok || len(batch) == 0 {
		return true
	}
	ids := w.changeSetIDs(oldest(batch))
	records := make([]changeRecord, 0, len(batch))
	for _, change := range batch {
		records = append(records, changeRecord{Change: change,
			ChangeSet: ids[changeKey(change.Entity.String(), change.At)]})
	}
	if err := store.AppendChanges(records); err != nil {
		w.failed(err, "changes")
		w.mu.Lock()
		w.changes = append(batch, w.changes...)
		w.trimChangesLocked()
		w.mu.Unlock()
		return false
	}
	return true
}

// flushHealthy writes the queued last-healthy marks. A failed write is
// kept, unless a newer mark of the entity came in meanwhile.
func (w *historyWriter) flushHealthy() bool {
	store, ok := w.store.(healthyStore)
	w.mu.Lock()
	batch := w.healthy
	w.healthy = nil
	w.mu.Unlock()
	if !ok || len(batch) == 0 {
		return true
	}
	if err := store.SaveHealthy(batch); err != nil {
		w.failed(err, "healthy")
		w.mu.Lock()
		if w.healthy == nil {
			w.healthy = map[string]healthyRecord{}
		}
		for entity, record := range batch {
			if _, newer := w.healthy[entity]; !newer {
				w.healthy[entity] = record
			}
		}
		w.mu.Unlock()
		return false
	}
	return true
}

// oldest is the earliest change time of changes.
func oldest(changes []inventory.Change) time.Time {
	first := changes[0].At
	for _, change := range changes {
		if change.At.Before(first) {
			first = change.At
		}
	}
	return first
}
