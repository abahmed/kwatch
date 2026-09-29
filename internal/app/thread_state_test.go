package app

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/knowledge/store"
)

type fakeThreads struct {
	mu       sync.Mutex
	threads  map[string]map[string]string
	restored []map[string]map[string]string
	events   []string
}

func (f *fakeThreads) SnapshotThreads() map[string]map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.threads
}

func (f *fakeThreads) RestoreThreads(m map[string]map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, "restore")
	f.restored = append(f.restored, m)
	f.threads = m
}

func (f *fakeThreads) set(m map[string]map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.threads = m
}

func threadDisk(t *testing.T) diskState {
	return diskState{store: openTestStore(t)}
}

var sampleThreads = map[string]map[string]string{
	"slack": {"p1": "1700.1", "p2": "1700.2"},
}

func TestThreadSaverRoundTripRestoresSameMap(t *testing.T) {
	disk := threadDisk(t)
	src := &fakeThreads{threads: sampleThreads}
	require.NoError(t, newThreadSaver(src, disk).Save())

	next := &fakeThreads{}
	newThreadSaver(next, disk).Restore()
	require.Equal(t, []map[string]map[string]string{sampleThreads},
		next.restored)
}

func TestThreadSaverRestoreRunsBeforeFirstSend(t *testing.T) {
	disk := threadDisk(t)
	require.NoError(t, newThreadSaver(
		&fakeThreads{threads: sampleThreads}, disk).Save())
	next := &fakeThreads{}
	newThreadSaver(next, disk).Restore()
	next.events = append(next.events, "send")
	require.Equal(t, []string{"restore", "send"}, next.events)
}

func TestThreadSaverSavesOnlyWhenChanged(t *testing.T) {
	disk := threadDisk(t)
	src := &fakeThreads{threads: sampleThreads}
	saver := newThreadSaver(src, disk)
	require.NoError(t, saver.Save())

	// Overwrite behind the saver's back: an unchanged snapshot must
	// not touch the store again.
	require.NoError(t, disk.put(stateProviderThreads, "marker"))
	require.NoError(t, saver.Save())
	var marker string
	_, err := disk.get(stateProviderThreads, &marker)
	require.NoError(t, err)
	require.Equal(t, "marker", marker)

	src.set(map[string]map[string]string{"slack": {"p1": "1"}})
	require.NoError(t, saver.Save())
	var rec threadRecord
	_, err = disk.get(stateProviderThreads, &rec)
	require.NoError(t, err)
	require.Equal(t, "1", rec.Threads["slack"]["p1"])
	require.Len(t, rec.Threads["slack"], 1)
}

func TestThreadSaverRemovalIsPersisted(t *testing.T) {
	disk := threadDisk(t)
	src := &fakeThreads{threads: sampleThreads}
	saver := newThreadSaver(src, disk)
	require.NoError(t, saver.Save())
	src.set(nil)
	require.NoError(t, saver.Save())
	next := &fakeThreads{}
	newThreadSaver(next, disk).Restore()
	require.Empty(t, next.restored)
}

func TestThreadSaverCorruptValueRestoresEmpty(t *testing.T) {
	disk := threadDisk(t)
	require.NoError(t, disk.put(stateProviderThreads, "not a record"))
	next := &fakeThreads{}
	require.NotPanics(t, func() {
		newThreadSaver(next, disk).Restore()
	})
	require.Empty(t, next.restored)
}

func TestThreadSaverFlushSavesAfterCancellation(t *testing.T) {
	disk := threadDisk(t)
	src := &fakeThreads{threads: sampleThreads}
	saver := newThreadSaver(src, disk)
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		defer close(done)
		saver.Run(ctx, tick)
	}()
	cancel()
	<-done
	saver.Flush(ctx)
	next := &fakeThreads{}
	newThreadSaver(next, disk).Restore()
	require.Equal(t, sampleThreads, next.threads)
}

func TestThreadSaverRunSavesOnTick(t *testing.T) {
	disk := threadDisk(t)
	saver := newThreadSaver(&fakeThreads{threads: sampleThreads}, disk)
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		defer close(done)
		saver.Run(ctx, tick)
	}()
	tick <- time.Time{}
	tick <- time.Time{} // returns only after the first save finished
	cancel()
	<-done
	var rec threadRecord
	found, err := disk.get(stateProviderThreads, &rec)
	require.NoError(t, err)
	require.True(t, found)
}

func TestThreadSaverStaleEpochWriteIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	opts := store.Options{Now: time.Now}
	newer, err := store.Open(path, opts)
	require.NoError(t, err)
	require.NoError(t, newer.Claim(2))
	require.NoError(t, newer.Close())

	stale, err := store.Open(path, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stale.Close() })
	require.ErrorIs(t, stale.Claim(1), store.ErrFenced)

	saver := newThreadSaver(&fakeThreads{threads: sampleThreads},
		diskState{store: stale})
	require.Error(t, saver.Save())
	require.NotPanics(t, func() { saver.Flush(context.Background()) })
	var rec threadRecord
	found, err := diskState{store: stale}.get(stateProviderThreads, &rec)
	require.NoError(t, err)
	require.False(t, found)
}
