package pipeline

import (
	"maps"
	"slices"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/storage"
)

// The timeline keeps the story of the issues that are still open when
// its caps evict (storage hold.go): the recorder holds the objects of
// every open incident, and marks the times of its decisions (opening,
// updates) on its root. A resolved incident's objects are let go, so
// their detail is evicted first.

// maxMarksPerEntity bounds the decision marks kept in memory per object,
// the earliest and the latest halves.
const maxMarksPerEntity = 32

// timelineHoldStore is the part of the store that holds timeline
// entities. Optional, so test stores need not implement it.
type timelineHoldStore interface {
	HoldTimeline(map[string]storage.LogHold) error
	HeldTimeline() (map[string]storage.LogHold, error)
}

// HoldTimeline implements timelineHoldStore.
func (d *diskIncidents) HoldTimeline(
	holds map[string]storage.LogHold,
) error {
	return d.timeline().Hold(holds)
}

// HeldTimeline implements timelineHoldStore.
func (d *diskIncidents) HeldTimeline() (map[string]storage.LogHold, error) {
	return d.timeline().Held()
}

// timelineHolds tracks the decision marks of the held objects; only the
// decision loop touches it.
type timelineHolds struct {
	marks map[string][]time.Time
}

// restore takes the marks the previous session held.
func (t *timelineHolds) restore(store HistoryStore) {
	saved, ok := store.(timelineHoldStore)
	if !ok {
		return
	}
	held, err := saved.HeldTimeline()
	if err != nil {
		klog.ErrorS(err, "pipeline: restore timeline holds",
			"component", "pipeline", "operation", "restore")
		return
	}
	t.marks = map[string][]time.Time{}
	for entity, hold := range held {
		t.marks[entity] = slices.Clone(hold.Marks)
	}
}

// decided marks the decisions on their roots and returns what to hold
// now: the objects of the open incidents among all, with their marks.
func (t *timelineHolds) decided(
	now time.Time, decisions []incident.Decision, all []incident.Incident,
) map[string]storage.LogHold {
	if t.marks == nil {
		t.marks = map[string][]time.Time{}
	}
	for _, d := range decisions {
		root := d.Incident.Root.String()
		t.marks[root] = boundedMarks(append(t.marks[root], now))
	}
	holds := map[string]storage.LogHold{}
	for _, p := range all {
		if p.State == incident.Resolved {
			continue
		}
		holds[p.Root.String()] = storage.LogHold{}
		for key := range p.Members {
			holds[key.Entity.String()] = storage.LogHold{}
		}
	}
	for entity := range maps.Keys(t.marks) {
		if _, open := holds[entity]; !open {
			delete(t.marks, entity)
			continue
		}
		holds[entity] = storage.LogHold{Marks: slices.Clone(t.marks[entity])}
	}
	return holds
}

// boundedMarks keeps the earliest and the latest marks.
func boundedMarks(marks []time.Time) []time.Time {
	if len(marks) <= maxMarksPerEntity {
		return marks
	}
	half := maxMarksPerEntity / 2
	return append(marks[:half:half], marks[len(marks)-half:]...)
}

// addHold queues what to hold; a newer value replaces the queued one.
func (w *historyWriter) addHold(holds map[string]storage.LogHold) {
	w.mu.Lock()
	w.hold = holds
	w.mu.Unlock()
	notify(w.wake)
}

// flushHold writes the queued hold. A failed write is kept unless a
// newer value came in meanwhile. It reports false on failure.
func (w *historyWriter) flushHold() bool {
	store, ok := w.store.(timelineHoldStore)
	w.mu.Lock()
	holds := w.hold
	w.hold = nil
	w.mu.Unlock()
	if !ok || holds == nil {
		return true
	}
	if err := store.HoldTimeline(holds); err != nil {
		w.failed(err, "timeline holds")
		w.mu.Lock()
		if w.hold == nil {
			w.hold = holds
		}
		w.mu.Unlock()
		return false
	}
	return true
}
