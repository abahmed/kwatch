package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/storage"
)

const (
	// threadKey holds the thread record in the store's threads bucket.
	threadKey          = "delivery.threads"
	threadSaveInterval = time.Minute
	threadFinalTimeout = 5 * time.Second
	threadStateVersion = 1
)

// threadSource is the delivery manager as the saver sees it.
type threadSource interface {
	SnapshotThreads() map[string]map[string]string
	RestoreThreads(map[string]map[string]string)
}

// threadRecord is the persisted value. Bump threadStateVersion on a
// layout change; unknown versions are ignored rather than misread.
type threadRecord struct {
	Version int                          `json:"version"`
	Threads map[string]map[string]string `json:"threads"`
}

// threadSaver persists provider thread ids so a restart continues open
// incidents in their existing threads. Writes are fenced by the store
// epoch, so a deposed leader cannot overwrite the new leader's state.
//
// mu serialises saves, so a Flush that timed out cannot race a later save
// on last/saved. closed stops every save once the session releases the
// store, including a timed-out save still waiting for mu.
type threadSaver struct {
	source threadSource
	disk   diskState
	mu     sync.Mutex
	closed atomic.Bool
	last   [sha256.Size]byte
	saved  bool
	// flushTimeout bounds Flush; it is threadFinalTimeout in production.
	flushTimeout time.Duration
	// wake asks Run for an immediate save; writes reports each write.
	wake   chan struct{}
	writes chan struct{}
}

func newThreadSaver(source threadSource, disk diskState) *threadSaver {
	return &threadSaver{
		source: source, disk: disk, flushTimeout: threadFinalTimeout,
		wake: make(chan struct{}, 1), writes: make(chan struct{}, 1),
	}
}

// Wake asks Run to save now. It never blocks and coalesces bursts, so
// the delivery path can call it after every accepted job. Run saves only
// when the snapshot changed, which makes a wake without news cheap.
func (t *threadSaver) Wake() {
	if t.closed.Load() {
		return
	}
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// Written receives one signal per successful write; it is the saver's
// progress signal for supervision and tests. Signals are dropped when
// nobody is listening.
func (t *threadSaver) Written() <-chan struct{} { return t.writes }

// Restore loads saved threads into the providers. A missing, corrupt or
// newer-format value yields an empty restore, never an error.
func (t *threadSaver) Restore() {
	t.mu.Lock()
	defer t.mu.Unlock()
	var rec threadRecord
	found, err := t.disk.getThreads(&rec)
	if err != nil {
		klog.ErrorS(err, "restore provider threads",
			"component", "threads", "operation", "restore")
		return
	}
	if !found || rec.Version != threadStateVersion ||
		len(rec.Threads) == 0 {
		return
	}
	t.source.RestoreThreads(rec.Threads)
	t.remember(rec.Threads)
}

func (t *threadSaver) remember(threads map[string]map[string]string) {
	if sum, ok := threadDigest(threads); ok {
		t.last, t.saved = sum, true
	}
}

// errThreadSaverClosed means the session released the store.
var errThreadSaverClosed = errors.New("thread saver closed")

// Save writes the current snapshot when it differs from the last write.
// It does not write once ctx has ended or the saver is closed.
func (t *threadSaver) Save(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed.Load() {
		return errThreadSaverClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	threads := t.source.SnapshotThreads()
	sum, ok := threadDigest(threads)
	if !ok || (t.saved && sum == t.last) {
		return nil
	}
	// The snapshot may have waited on the provider; recheck before writing.
	if t.closed.Load() {
		return errThreadSaverClosed
	}
	rec := threadRecord{Version: threadStateVersion, Threads: threads}
	if err := t.disk.putThreads(rec); err != nil {
		return err
	}
	t.last, t.saved = sum, true
	select {
	case t.writes <- struct{}{}:
	default:
	}
	return nil
}

// Close stops all further saves. It never waits for a save in progress.
func (t *threadSaver) Close() {
	t.closed.Store(true)
}

func threadDigest(
	threads map[string]map[string]string,
) ([sha256.Size]byte, bool) {
	data, err := json.Marshal(threads)
	if err != nil {
		return [sha256.Size]byte{}, false
	}
	return sha256.Sum256(data), true
}

// Run saves on every tick and on every Wake until ctx ends. A first
// thread id is therefore written as soon as its provider accepted the
// post, not up to a minute later; a crash in between would otherwise let
// the next leader open a duplicate thread. It never writes after
// cancellation; Flush performs the final bounded save.
func (t *threadSaver) Run(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			t.logSave(ctx)
		case <-t.wake:
			t.saveBounded(ctx)
		}
	}
}

// saveBounded is the immediate save; the store write cannot outlive
// threadFinalTimeout even if the disk stalls.
func (t *threadSaver) saveBounded(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, t.flushTimeout)
	defer cancel()
	t.logSave(ctx)
}

// Flush is the shutdown save, bounded by flushTimeout. It reports
// whether the save returned in time; a save that did not keeps running
// until its store write returns, and the caller must not close the store.
func (t *threadSaver) Flush(parent context.Context) bool {
	ctx, cancel := context.WithTimeout(
		context.WithoutCancel(parent), t.flushTimeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		t.logSave(ctx)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		klog.InfoS("provider thread save timed out",
			"component", "threads", "operation", "flush")
		return false
	}
}

func (t *threadSaver) logSave(ctx context.Context) {
	if err := t.Save(ctx); err != nil && ctx.Err() == nil {
		klog.ErrorS(err, "save provider threads",
			"component", "threads", "operation", "save",
			"fenced", isFenced(err))
	}
}

// getThreads reads the saved thread record into out.
func (d diskState) getThreads(out any) (bool, error) {
	return getRaw(storage.ThreadValues[json.RawMessage](d.store),
		threadKey, out)
}

// putThreads replaces the saved thread record.
func (d diskState) putThreads(value any) error {
	return putRaw(storage.ThreadValues[json.RawMessage](d.store),
		threadKey, value)
}

func isFenced(err error) bool {
	return errors.Is(err, storage.ErrFenced)
}

func runThreadSaver(saver *threadSaver) func(context.Context) error {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(threadSaveInterval)
		defer ticker.Stop()
		saver.Run(ctx, ticker.C)
		return nil
	}
}
