package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/knowledge/store"
)

const (
	stateProviderThreads = "delivery.threads"
	threadSaveInterval   = time.Minute
	threadFinalTimeout   = 5 * time.Second
	threadStateVersion   = 1
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
// problems in their existing threads. Writes are fenced by the store
// epoch, so a deposed leader cannot overwrite the new leader's state.
type threadSaver struct {
	source threadSource
	disk   diskState
	last   [sha256.Size]byte
	saved  bool
}

func newThreadSaver(source threadSource, disk diskState) *threadSaver {
	return &threadSaver{source: source, disk: disk}
}

// Restore loads saved threads into the providers. A missing, corrupt or
// newer-format value yields an empty restore, never an error.
func (t *threadSaver) Restore() {
	var rec threadRecord
	found, err := t.disk.get(stateProviderThreads, &rec)
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

// Save writes the current snapshot when it differs from the last write.
func (t *threadSaver) Save() error {
	threads := t.source.SnapshotThreads()
	sum, ok := threadDigest(threads)
	if !ok || (t.saved && sum == t.last) {
		return nil
	}
	rec := threadRecord{Version: threadStateVersion, Threads: threads}
	if err := t.disk.put(stateProviderThreads, rec); err != nil {
		return err
	}
	t.last, t.saved = sum, true
	return nil
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

// Run saves on every tick until ctx ends. It never writes after
// cancellation; Flush performs the final bounded save.
func (t *threadSaver) Run(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			t.logSave()
		}
	}
}

// Flush is the shutdown save, bounded by threadFinalTimeout.
func (t *threadSaver) Flush(ctx context.Context) {
	ctx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx), threadFinalTimeout)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		t.logSave()
	}()
	select {
	case <-done:
	case <-ctx.Done():
		klog.InfoS("provider thread save timed out",
			"component", "threads", "operation", "flush")
	}
}

func (t *threadSaver) logSave() {
	if err := t.Save(); err != nil {
		klog.ErrorS(err, "save provider threads",
			"component", "threads", "operation", "save",
			"fenced", isFenced(err))
	}
}

func isFenced(err error) bool {
	return errors.Is(err, store.ErrFenced)
}

func runThreadSaver(saver *threadSaver) func(context.Context) error {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(threadSaveInterval)
		defer ticker.Stop()
		saver.Run(ctx, ticker.C)
		return nil
	}
}
