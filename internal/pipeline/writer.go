package pipeline

import (
	"errors"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/storage"
)

// writeBatch is how long the storage writer collects snapshots before it
// writes the latest one. Everything decided within a batch costs one write.
const writeBatch = time.Second

// storeSnapshot is the state to persist. The decision loop builds it and
// never touches it again, so the writer can read it without locks. A nil
// part means "nothing new to write".
type storeSnapshot struct {
	incidents    []incident.Record
	hasIncidents bool
	fingerprints map[string]any
	startup      *announce.StartupState
}

// replaceWith overwrites every part that newer carries: only the latest
// value of each part is worth writing.
func (s *storeSnapshot) replaceWith(newer storeSnapshot) {
	if newer.hasIncidents {
		s.incidents, s.hasIncidents = newer.incidents, true
	}
	if newer.fingerprints != nil {
		s.fingerprints = newer.fingerprints
	}
	if newer.startup != nil {
		s.startup = newer.startup
	}
}

// keepOlder puts back the parts of older that failed to write, unless a
// newer value arrived meanwhile.
func (s *storeSnapshot) keepOlder(older storeSnapshot) {
	if older.hasIncidents && !s.hasIncidents {
		s.incidents, s.hasIncidents = older.incidents, true
	}
	if older.fingerprints != nil && s.fingerprints == nil {
		s.fingerprints = older.fingerprints
	}
	if older.startup != nil && s.startup == nil {
		s.startup = older.startup
	}
}

func (s storeSnapshot) empty() bool {
	return !s.hasIncidents && s.fingerprints == nil && s.startup == nil
}

// storeWriter is the only goroutine that writes the IncidentStore while
// the engine runs. The loop offers snapshots and never waits; the writer
// keeps one pending snapshot, so its queue is bounded by construction.
type storeWriter struct {
	store IncidentStore
	after func(time.Duration) <-chan time.Time
	stats *workerStats

	mu      sync.Mutex
	pending storeSnapshot
	// fenced is set once a write found the store claimed by a newer
	// leader. Fencing is terminal: no later write can succeed, so the
	// writer stops and drops what is pending.
	fenced bool

	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
	stopped sync.Once
}

func newStoreWriter(
	store IncidentStore, after func(time.Duration) <-chan time.Time,
	stats *workerStats,
) *storeWriter {
	return &storeWriter{
		store: store, after: after, stats: stats,
		wake: make(chan struct{}, 1),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// offer queues s for the next batch. It never blocks. A fenced writer
// drops s: nothing can be written any more.
func (w *storeWriter) offer(s storeSnapshot) {
	w.mu.Lock()
	if w.fenced {
		w.mu.Unlock()
		return
	}
	w.pending.replaceWith(s)
	w.mu.Unlock()
	notify(w.wake)
}

// run writes a batch at most once per writeBatch until close is called,
// then writes whatever is still pending and returns. It returns early,
// for good, once the store is fenced.
func (w *storeWriter) run() {
	defer close(w.done)
	for !w.isFenced() {
		select {
		case <-w.stop:
			w.finalFlush()
			return
		case <-w.wake:
		}
		select {
		case <-w.stop:
			w.finalFlush()
			return
		case <-w.after(writeBatch):
		}
		w.flush()
	}
}

// fingerprintFlusher is a store that holds fingerprint writes back
// between intervals (see diskIncidents).
type fingerprintFlusher interface {
	FlushFingerprints() error
}

// finalFlush writes what is pending, then any fingerprints the store
// held back. Writing them here, not only when the store closes, means a
// process killed after this final write (out of memory, a shutdown
// timeout) still leaves the latest fingerprints: the next start does
// not mistake recent changes for changes made while kwatch was down.
func (w *storeWriter) finalFlush() {
	w.flush()
	flusher, ok := w.store.(fingerprintFlusher)
	if !ok || w.isFenced() {
		return
	}
	err := flusher.FlushFingerprints()
	switch {
	case isFenced(err):
		w.stopFenced()
	case err != nil:
		logWriteError(err, "fingerprints")
		w.stats.writeFailures.Add(1)
		metrics.DefaultRegistry().StorageWriteFailures.Add(1)
	}
}

func (w *storeWriter) isFenced() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fenced
}

// askStop asks the writer for its final write without waiting.
func (w *storeWriter) askStop() {
	w.stopped.Do(func() { close(w.stop) })
}

// close asks the writer for its final write and waits at most for the
// deadline channel. It reports whether the writer finished in time.
func (w *storeWriter) close(deadline <-chan time.Time) bool {
	w.askStop()
	select {
	case <-w.done:
		return true
	case <-deadline:
		return false
	}
}

// flush writes the pending snapshot. Parts that fail are put back and
// retried with the next batch.
func (w *storeWriter) flush() {
	w.mu.Lock()
	batch := w.pending
	w.pending = storeSnapshot{}
	w.mu.Unlock()
	if batch.empty() {
		return
	}
	failed, fenced := w.write(batch)
	w.stats.batches.Add(1)
	if fenced {
		w.stopFenced()
		return
	}
	if failed.empty() {
		return
	}
	w.stats.writeFailures.Add(1)
	metrics.DefaultRegistry().StorageWriteFailures.Add(1)
	w.mu.Lock()
	w.pending.keepOlder(failed)
	w.mu.Unlock()
	notify(w.wake)
}

// write stores each part of batch and returns the parts that failed.
// fenced reports that the store was claimed by a newer leader; the
// remaining parts are not tried and nothing is retried.
func (w *storeWriter) write(
	batch storeSnapshot,
) (_ storeSnapshot, fenced bool) {
	var failed storeSnapshot
	if batch.startup != nil {
		err := w.store.SaveStartup(*batch.startup)
		if isFenced(err) {
			return storeSnapshot{}, true
		}
		if err != nil {
			logWriteError(err, "startup marker")
			failed.startup = batch.startup
		}
	}
	if batch.hasIncidents {
		err := w.store.SaveIncidents(batch.incidents)
		if isFenced(err) {
			return storeSnapshot{}, true
		}
		if err != nil {
			logWriteError(err, "incidents")
			failed.incidents, failed.hasIncidents = batch.incidents, true
		}
	}
	if batch.fingerprints != nil {
		err := w.store.SaveFingerprints(batch.fingerprints)
		if isFenced(err) {
			return storeSnapshot{}, true
		}
		if err != nil {
			logWriteError(err, "fingerprints")
			failed.fingerprints = batch.fingerprints
		}
	}
	return failed, false
}

func isFenced(err error) bool {
	return errors.Is(err, storage.ErrFenced)
}

// stopFenced makes the writer terminal and logs it once. A newer leader
// owns the store; the supervisor of this term handles the lost lease.
func (w *storeWriter) stopFenced() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fenced {
		return
	}
	w.fenced, w.pending = true, storeSnapshot{}
	w.stats.writeFailures.Add(1)
	metrics.DefaultRegistry().StorageWriteFailures.Add(1)
	klog.InfoS("pipeline: store claimed by a newer leader; "+
		"storage writer stopped", "component", "pipeline",
		"operation", "save")
}

func logWriteError(err error, what string) {
	klog.ErrorS(err, "pipeline: storage write failed; retrying next batch",
		"component", "pipeline", "operation", "save", "data", what)
}
