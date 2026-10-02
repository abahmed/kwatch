package pipeline

import (
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// historyRecorder feeds what the loop sees into the model's history
// (health marks, baselines) and, with a HistoryStore, into the persisted
// timeline. Every method runs on the decision loop and only touches
// memory; the writer does the I/O.
type historyRecorder struct {
	model   *inventory.Model
	sampler *baselineSampler
	store   HistoryStore
	timer   func(time.Duration) <-chan time.Time
	now     func() time.Time
	// writer is nil without a HistoryStore, and before start.
	writer *historyWriter
	stats  *workerStats
}

func newHistoryRecorder(
	deps Dependencies, stats *workerStats,
) *historyRecorder {
	store, _ := deps.Store.(HistoryStore)
	// Without a clock, as in unit tests, no time passes: restore keeps
	// every hour window and nothing expires.
	now := func() time.Time { return time.Time{} }
	if deps.Clock != nil {
		now = deps.Clock.Now
	}
	return &historyRecorder{
		model: deps.Model, sampler: newBaselineSampler(deps.Model),
		store: store, timer: deps.Timer, now: now, stats: stats,
	}
}

// start restores the baselines and starts the writer. It runs before the
// loop, so it may read the store directly. stop ends the writer.
func (h *historyRecorder) start() {
	if h.store == nil {
		return
	}
	saved, err := h.store.LoadBaselines()
	if err != nil {
		klog.ErrorS(err, "pipeline: restore baselines; starting empty",
			"component", "pipeline", "operation", "restore")
	}
	h.model.Baselines().Restore(saved, h.now())
	h.writer = newHistoryWriter(h.store, h.model, h.timer, h.stats)
	h.writer.expireFrom(h.now)
	go h.writer.run()
}

// askStop asks the writer for its last batch without waiting.
func (h *historyRecorder) askStop() {
	if h.writer != nil {
		h.writer.askStop()
	}
}

// stop asks the writer for its last batch and waits at most until
// deadline. It reports whether the writer finished in time.
func (h *historyRecorder) stop(deadline <-chan time.Time) bool {
	return h.writer == nil || h.writer.close(deadline)
}

// observed records one applied observation.
func (h *historyRecorder) observed(o inventory.Observation) {
	h.sampler.observe(o)
	if h.writer == nil {
		return
	}
	switch o.Kind {
	case inventory.Changed:
		change := o.Change
		change.Entity = o.Entity
		if change.At.IsZero() {
			change.At = o.At
		}
		h.writer.add(changeEntry(change))
	case inventory.Noted:
		if o.Note.Warning {
			h.writer.add(eventEntry(o.Entity, h.storedNote(o), o.At))
		}
	}
}

// storedNote returns the note as the model keeps it, with its first-seen
// time merged from earlier repeats.
func (h *historyRecorder) storedNote(o inventory.Observation) inventory.Note {
	notes := h.model.Notes(o.Entity, time.Time{})
	for i := len(notes) - 1; i >= 0; i-- {
		if notes[i].Source == o.Note.Source && notes[i].Reason == o.Note.Reason {
			return notes[i]
		}
	}
	return o.Note
}

// transitions records finding transitions as timeline entries, and one
// health mark per entity they touched. The mark reads the entity's
// active findings after the transitions, through active: clearing one
// finding of an entity that still has another failing one leaves it
// failing. Findings of unknown health say nothing either way: while one
// is active and nothing fails, no mark is written, since kwatch cannot
// tell that the entity is healthy.
func (h *historyRecorder) transitions(
	now time.Time, transitions []detection.Transition,
	active func(inventory.EntityID) []detection.Finding,
) {
	var entries []TimelineEntry
	marked := map[inventory.EntityID]bool{}
	for _, t := range transitions {
		s := t.Finding
		entries = append(entries, findingEntry(t, now))
		if marked[s.Entity] {
			continue
		}
		marked[s.Entity] = true
		failing, mode, unknown := healthMark(active(s.Entity))
		if !failing && unknown {
			continue
		}
		if !failing {
			mode = string(s.Mode)
		}
		h.model.MarkHealth(s.Entity, failing, mode, now)
	}
	if h.writer != nil {
		h.writer.add(entries...)
	}
}

// healthMark reports whether any active finding is failing or degraded,
// the mode of the worst one, and whether a finding of unknown health is
// active. Healthy findings are skipped.
func healthMark(
	active []detection.Finding,
) (failing bool, mode string, unknown bool) {
	worst := detection.Health(0)
	for _, f := range active {
		if f.Health == detection.Unknown {
			unknown = true
		}
		if f.Health != detection.Failing && f.Health != detection.Degraded {
			continue
		}
		if f.Health > worst {
			worst, mode = f.Health, string(f.Mode)
		}
	}
	return worst != 0, mode, unknown
}

// decided records kwatch's decisions on their incidents' roots.
func (h *historyRecorder) decided(
	now time.Time, decisions []incident.Decision,
) {
	if h.writer == nil {
		return
	}
	for _, d := range decisions {
		h.writer.add(decisionEntry(d, now))
	}
}

// stopped reports whether the history writer has finished, so the store
// is not closed while a timeline or baseline write is still running.
func (h *historyRecorder) stopped() bool {
	if h.writer == nil {
		return true
	}
	select {
	case <-h.writer.done:
		return true
	default:
		return false
	}
}
