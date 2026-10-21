package flight

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/safego"
)

// Bounds of the recorder. Memory is at most 2*maxPending queue slots (the
// queue holds maxPending entries; see trimPending) and maxKeys remembered
// states, whatever the cluster does.
const (
	// flushEvery is how often the queue is written.
	flushEvery = 5 * time.Second
	// maxPending bounds the queue; when it is full the oldest entries go.
	maxPending = 8192
	// maxKeys bounds the states remembered to skip repeats; the memory
	// is cleared when it is full, which only records a few repeats.
	maxKeys = 20000
	// refreshEvery is how long an unchanged state is skipped before it
	// is recorded again, so a window always starts from a known state.
	refreshEvery = 10 * time.Minute
)

// Submit is the signature of the pipeline's Submit.
type Submit func(context.Context, ...inventory.Observation)

// Recorder sits between the sources and the engine. It forwards every
// observation untouched, then sanitizes a copy and queues it; a writer
// stores the queue in batches. It never blocks and never fails the
// pipeline: recording is best effort and bounded.
type Recorder struct {
	sanitizer *replay.Sanitizer
	ring      Ring
	now       func() time.Time

	mu      sync.Mutex
	pending []replay.Entry
	skipped int // slots dropped from the front of pending's array
	last    time.Time
	dropped int

	// seen is only touched by Flush, under flushMu.
	flushMu sync.Mutex
	seen    map[string]seenState
	failed  int
}

type seenState struct {
	sum [sha256.Size]byte
	at  time.Time
}

// NewRecorder returns a recorder that writes to ring, with pseudonyms
// keyed by salt. now is the pipeline clock.
func NewRecorder(
	ring Ring, salt string, now func() time.Time,
) (*Recorder, error) {
	sanitizer, err := replay.NewSanitizer(replay.SanitizeOptions{Salt: salt})
	if err != nil {
		return nil, err
	}
	return &Recorder{
		sanitizer: sanitizer, ring: ring, now: now,
		seen: map[string]seenState{},
	}, nil
}

// Wrap returns a Submit that records what it forwards to next.
func (r *Recorder) Wrap(next Submit) Submit {
	return func(ctx context.Context, observations ...inventory.Observation) {
		next(ctx, observations...)
		r.record(observations)
	}
}

// record sanitizes observations and queues them under one timestamp.
func (r *Recorder) record(observations []inventory.Observation) {
	clean := make([]inventory.Observation, len(observations))
	for i, o := range observations {
		clean[i] = r.sanitizer.Observation(o)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	at := r.now().UTC()
	if at.Before(r.last) {
		at = r.last // the wall clock stepped back; keep the log ordered
	}
	r.last = at
	for _, o := range clean {
		r.pending = append(r.pending, replay.Entry{At: at, Observation: o})
	}
	r.trimPending()
}

// trimPending drops the oldest entries beyond maxPending. It reslices the
// queue instead of copying it, so a full queue costs the same per call as
// an empty one. The dropped entries stay as unused slots at the front of
// the backing array; once they are as many as the queue holds, the live
// entries move to a fresh array, so memory never passes 2*maxPending slots.
func (r *Recorder) trimPending() {
	over := len(r.pending) - maxPending
	if over <= 0 {
		return
	}
	clear(r.pending[:over]) // let the dropped observations be collected
	r.pending = r.pending[over:]
	r.dropped += over
	r.skipped += over
	if r.skipped >= maxPending {
		fresh := make([]replay.Entry, len(r.pending), 2*maxPending)
		copy(fresh, r.pending)
		r.pending, r.skipped = fresh, 0
	}
}

// Pending is the number of queued entries.
func (r *Recorder) Pending() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pending)
}

// Dropped is how many entries were dropped because the queue was full.
func (r *Recorder) Dropped() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

// Run flushes every flushEvery until ctx ends, then once more.
func (r *Recorder) Run(ctx context.Context) {
	ticker := time.NewTicker(flushEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.safeFlush()
			return
		case <-ticker.C:
			r.safeFlush()
		}
	}
}

// safeFlush is Flush with a panic contained: recording is best effort and
// must never end the process. The batch being written is lost.
func (r *Recorder) safeFlush() {
	safego.Do("flight-flush", r.Flush)
}

// Flush writes the queue now. Repeats of a recorded state are skipped. A
// failed write is dropped, not retried: the ring is a best-effort trail,
// and a retry queue would be one more thing to bound.
func (r *Recorder) Flush() {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()
	r.mu.Lock()
	batch, dropped := r.pending, r.dropped
	r.pending, r.skipped = nil, 0
	r.mu.Unlock()
	entries, marks := r.withoutRepeats(batch)
	for len(entries) > 0 {
		n := min(len(entries), maxBatchEntries)
		if err := r.ring.Append(entries[:n]); err != nil {
			r.failed += n
			klog.V(1).InfoS("flight: write failed; entries dropped",
				"component", "flight", "operation", "write", "error", err)
		} else {
			r.remember(marks[:n])
		}
		entries, marks = entries[n:], marks[n:]
	}
	if dropped > 0 || r.failed > 0 {
		klog.V(2).InfoS("flight: recording is losing entries",
			"component", "flight", "queueDropped", dropped,
			"writeFailed", r.failed)
	}
}

// seenMark is the state to remember for a key once its entry is written.
type seenMark struct {
	key   string
	state seenState
	ok    bool
}

// withoutRepeats drops the Observed and Related entries that repeat the
// state recorded for the same key less than refreshEvery ago. It returns
// a mark for each kept entry; the state is remembered only after the
// entry is written, so a failed write never hides the next observation.
func (r *Recorder) withoutRepeats(
	in []replay.Entry,
) ([]replay.Entry, []seenMark) {
	out := in[:0:0]
	var marks []seenMark
	batch := map[string]seenState{}
	for _, entry := range in {
		mark, skip := r.repeat(entry, batch)
		if !skip {
			out = append(out, entry)
			marks = append(marks, mark)
		}
	}
	return out, marks
}

// remember records the written entries' states.
func (r *Recorder) remember(marks []seenMark) {
	for _, m := range marks {
		if !m.ok {
			continue
		}
		if _, known := r.seen[m.key]; !known && len(r.seen) >= maxKeys {
			r.seen = map[string]seenState{}
		}
		r.seen[m.key] = m.state
	}
}

// repeat reports an entry that repeats its key's recorded state. Keys
// already kept in this batch (not yet written) count as recorded.
func (r *Recorder) repeat(
	entry replay.Entry, batch map[string]seenState,
) (seenMark, bool) {
	o := entry.Observation
	if o.Kind != inventory.Observed && o.Kind != inventory.Related {
		return seenMark{}, false
	}
	key := o.Kind.String() + "|" + o.Source + "|" + o.Entity.String() +
		"|" + string(o.Relation)
	o.At = time.Time{}
	data, err := json.Marshal(o)
	if err != nil {
		return seenMark{}, false
	}
	state := seenState{sum: sha256.Sum256(data), at: entry.At}
	prev, known := batch[key]
	if !known {
		prev, known = r.seen[key]
	}
	if known && prev.sum == state.sum &&
		entry.At.Sub(prev.at) < refreshEvery {
		return seenMark{}, true
	}
	batch[key] = state
	return seenMark{key, state, true}, false
}

// ErrNoRecording is returned by Export when the window holds nothing.
var ErrNoRecording = errors.New("flight: no recording in that window")
