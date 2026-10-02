package pipeline

import (
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/metrics"
)

// maxPendingTimeline bounds timeline entries waiting for the writer. When
// the store is slow or failing, the oldest entries are dropped first.
const maxPendingTimeline = 4096

// baselineExpiryInterval is how often the writer looks for expired
// baselines.
const baselineExpiryInterval = 10 * time.Minute

// historyWriter is the only goroutine that writes the timeline and the
// baselines. The loop adds entries and never waits; the writer writes
// at most once per writeBatch, like the incident writer.
type historyWriter struct {
	store HistoryStore
	model *inventory.Model
	after func(time.Duration) <-chan time.Time
	stats *workerStats

	mu      sync.Mutex
	pending []TimelineEntry
	dropped int

	// now and nextExpiry schedule baseline expiry. Only the writer
	// goroutine reads them, so expiry deletes and baseline saves never
	// race: a baseline saved after it expired is a new sample.
	now        func() time.Time
	nextExpiry time.Time

	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
	stopped sync.Once
}

func newHistoryWriter(
	store HistoryStore, model *inventory.Model,
	after func(time.Duration) <-chan time.Time, stats *workerStats,
) *historyWriter {
	return &historyWriter{
		store: store, model: model, after: after, stats: stats,
		wake: make(chan struct{}, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// expireFrom turns on baseline expiry, read from now. The first expiry
// waits one baselineExpiryInterval, so the model has time to list the
// workloads that still exist. Call it before run.
func (w *historyWriter) expireFrom(now func() time.Time) {
	w.now = now
	w.nextExpiry = now().Add(baselineExpiryInterval)
}

// add queues entries for the next batch. It never blocks.
func (w *historyWriter) add(entries ...TimelineEntry) {
	if len(entries) == 0 {
		return
	}
	w.mu.Lock()
	w.pending = append(w.pending, entries...)
	w.trimLocked()
	w.mu.Unlock()
	notify(w.wake)
}

func (w *historyWriter) trimLocked() {
	if over := len(w.pending) - maxPendingTimeline; over > 0 {
		w.pending = append(w.pending[:0:0], w.pending[over:]...)
		w.dropped += over
	}
}

// run writes a batch at most once per writeBatch until close is called,
// then writes whatever is still pending and returns. Baselines change on
// every observation, so the writer also wakes once per batch interval
// while anything could be dirty.
func (w *historyWriter) run() {
	defer close(w.done)
	for {
		select {
		case <-w.stop:
			w.flush()
			return
		case <-w.wake:
		case <-w.after(saveInterval):
		}
		select {
		case <-w.stop:
			w.flush()
			return
		case <-w.after(writeBatch):
		}
		w.flush()
	}
}

// close asks for the final write and waits at most for deadline. It
// reports whether the writer finished in time.
func (w *historyWriter) close(deadline <-chan time.Time) bool {
	w.askStop()
	select {
	case <-w.done:
		return true
	case <-deadline:
		return false
	}
}

// askStop asks for the final write without waiting for it.
func (w *historyWriter) askStop() {
	w.stopped.Do(func() { close(w.stop) })
}

// flush writes the pending timeline and the dirty baselines. A failed
// part is kept for the next batch.
func (w *historyWriter) flush() {
	w.mu.Lock()
	batch, dropped := w.pending, w.dropped
	w.pending, w.dropped = nil, 0
	w.mu.Unlock()
	if dropped > 0 {
		klog.InfoS("pipeline: timeline entries dropped while storage lagged",
			"component", "pipeline", "operation", "timeline", "count", dropped)
	}
	failed := false
	if len(batch) > 0 {
		w.fillChangeSets(batch)
		if err := w.store.AppendTimeline(batch); err != nil {
			logWriteError(err, "timeline")
			w.requeue(batch)
			failed = true
		}
	}
	if !w.expireBaselines() {
		failed = true
	}
	if dirty := w.model.Baselines().TakeDirty(); len(dirty) > 0 {
		if err := w.store.SaveBaselines(dirty); err != nil {
			logWriteError(err, "baselines")
			w.model.Baselines().MarkDirty(dirty)
			failed = true
		}
	}
	if len(batch) > 0 || failed {
		w.stats.batches.Add(1)
	}
	if failed {
		w.stats.writeFailures.Add(1)
		metrics.DefaultRegistry().StorageWriteFailures.Add(1)
	}
}

// baselineDeleter is the part of the store that removes stored
// baselines. It is optional so test stores need not implement it.
type baselineDeleter interface {
	DeleteBaselines(keys []string) error
}

// expireBaselines forgets, once per baselineExpiryInterval, baselines of
// gone workloads that got no sample for inventory.BaselineTTL, in memory
// and in the store. It reports false when the store delete failed; the
// keys are then left in the store until the next restart expires them
// again.
func (w *historyWriter) expireBaselines() bool {
	if w.now == nil {
		return true
	}
	now := w.now()
	if now.Before(w.nextExpiry) {
		return true
	}
	w.nextExpiry = now.Add(baselineExpiryInterval)
	expired := w.model.Baselines().Expired(now, w.model.Exists)
	deleter, ok := w.store.(baselineDeleter)
	if len(expired) == 0 || !ok {
		return true
	}
	if err := deleter.DeleteBaselines(expired); err != nil {
		logWriteError(err, "baseline_expiry")
		return false
	}
	return true
}

// requeue puts a failed batch back in front of newer entries.
func (w *historyWriter) requeue(batch []TimelineEntry) {
	w.mu.Lock()
	w.pending = append(batch, w.pending...)
	w.trimLocked()
	w.mu.Unlock()
}

// fillChangeSets names the change set of every change entry. It builds
// the sets once per batch, here rather than on the decision loop.
func (w *historyWriter) fillChangeSets(batch []TimelineEntry) {
	var since time.Time
	for _, entry := range batch {
		if entry.Kind == TimelineChange &&
			(since.IsZero() || entry.At.Before(since)) {
			since = entry.At
		}
	}
	if since.IsZero() {
		return
	}
	ids := make(map[string]string)
	for _, set := range w.model.RecentChangeSets(since) {
		for _, change := range set.Changes {
			ids[changeKey(change.Entity.String(), change.At)] = set.ID
		}
	}
	for i := range batch {
		if batch[i].Kind == TimelineChange && batch[i].ChangeSet == "" {
			key := changeKey(batch[i].Entity, batch[i].changeAt)
			batch[i].ChangeSet = ids[key]
		}
	}
}

func changeKey(entity string, at time.Time) string {
	return entity + "@" + at.UTC().Format(time.RFC3339Nano)
}
