package pipeline

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// batchTimer hands the writer a batch channel the test fires, and reports
// every batch wait on waits, so a test knows the writer is idle.
type batchTimer struct {
	fire  chan time.Time
	waits chan struct{}
}

func newBatchTimer() *batchTimer {
	return &batchTimer{
		fire: make(chan time.Time), waits: make(chan struct{}, 16),
	}
}

func (b *batchTimer) after(time.Duration) <-chan time.Time {
	b.waits <- struct{}{}
	return b.fire
}

func records(ids ...string) storeSnapshot {
	out := storeSnapshot{hasIncidents: true}
	for _, id := range ids {
		out.incidents = append(out.incidents, incident.Record{ID: id})
	}
	return out
}

func startWriter(
	t *testing.T, st *memStore, timer *batchTimer,
) *storeWriter {
	t.Helper()
	w := newStoreWriter(st, timer.after, &workerStats{})
	go w.run()
	t.Cleanup(func() { w.close(nil) })
	return w
}

func TestStoreWriterBatchesSnapshotsIntoOneWrite(t *testing.T) {
	st := &memStore{}
	timer := newBatchTimer()
	w := startWriter(t, st, timer)

	w.offer(records("a"))
	waitFor(t, timer.waits)
	w.offer(records("a", "b"))
	w.offer(records("a", "b", "c"))
	timer.fire <- time.Time{}
	waitFor(t, timer.waits)
	w.offer(records("d"))

	if !w.close(nil) {
		t.Fatal("close must finish")
	}
	if saved, _ := st.counts(); saved != 2 {
		t.Fatalf("writes = %d, want one batch plus the final flush", saved)
	}
	if got := st.latest(); len(got) != 1 || got[0].ID != "d" {
		t.Fatalf("latest write = %+v, want the newest snapshot", got)
	}
	if n := w.stats.batches.Load(); n != 2 {
		t.Fatalf("batches = %d, want 2", n)
	}
}

func TestStoreWriterRetriesFailedWriteNextBatch(t *testing.T) {
	st := &memStore{saveErr: errStore}
	timer := newBatchTimer()
	w := startWriter(t, st, timer)

	w.offer(records("a"))
	waitFor(t, timer.waits)
	timer.fire <- time.Time{}
	waitFor(t, timer.waits)
	st.setSaveErr(nil)
	timer.fire <- time.Time{}
	w.close(nil)

	if saved, _ := st.counts(); saved != 2 {
		t.Fatalf("writes = %d, want the failure and its retry", saved)
	}
	if got := st.latest(); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("retried write = %+v", got)
	}
	if n := w.stats.writeFailures.Load(); n != 1 {
		t.Fatalf("failures = %d, want 1", n)
	}
}

func TestStoreWriterRetryKeepsNewerSnapshot(t *testing.T) {
	var pending storeSnapshot
	pending.replaceWith(records("new"))

	pending.keepOlder(records("old"))

	if pending.incidents[0].ID != "new" {
		t.Fatalf("a failed older write replaced a newer snapshot")
	}
}

func TestStoreWriterFlushesPendingOnClose(t *testing.T) {
	st := &memStore{}
	timer := newBatchTimer()
	w := startWriter(t, st, timer)
	state := StartupState{Complete: true}

	w.offer(records("a"))
	w.offer(storeSnapshot{startup: &state})
	w.offer(storeSnapshot{fingerprints: map[string]any{"k": "v"}})

	if !w.close(nil) {
		t.Fatal("close must finish")
	}
	saved, prints := st.counts()
	if saved != 1 || prints != 1 || len(st.startups) != 1 {
		t.Fatalf("final flush wrote %d/%d/%d, want every part once",
			saved, prints, len(st.startups))
	}
}

func TestStoreWriterCloseGivesUpAtDeadline(t *testing.T) {
	st := &memStore{gate: make(chan struct{}),
		entered: make(chan struct{}, 1)}
	w := newStoreWriter(st, newBatchTimer().after, &workerStats{})
	go w.run()
	w.offer(records("a"))
	expired := make(chan time.Time, 1)
	expired <- time.Time{}

	if w.close(expired) {
		t.Fatal("close must give up while the store is stuck")
	}
	<-st.entered
	close(st.gate)
	waitFor(t, w.done)
}

func TestStoreWriterIgnoresEmptyBatches(t *testing.T) {
	st := &memStore{}
	w := newStoreWriter(st, newBatchTimer().after, &workerStats{})

	w.flush()

	if saved, _ := st.counts(); saved != 0 || w.stats.batches.Load() != 0 {
		t.Fatal("an empty batch must not write")
	}
}
