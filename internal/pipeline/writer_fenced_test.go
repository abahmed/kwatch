package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/storage"
)

// A fenced store never accepts a write again: the writer stops for good
// after the first fenced write instead of retrying every batch.
func TestStoreWriterStopsForGoodWhenFenced(t *testing.T) {
	st := &memStore{saveErr: fmt.Errorf("save: %w", storage.ErrFenced)}
	timer := newBatchTimer()
	w := newStoreWriter(st, timer.after, &workerStats{})
	go w.run()

	w.offer(records("a"))
	waitFor(t, timer.waits)
	timer.fire <- time.Time{}
	select {
	case <-w.done:
	case <-time.After(5 * time.Second):
		t.Fatal("fenced writer must stop on its own")
	}

	w.offer(records("b"))
	if !w.close(nil) {
		t.Fatal("close of a stopped writer must finish")
	}
	if saved, _ := st.counts(); saved != 1 {
		t.Fatalf("writes = %d, want only the fenced one", saved)
	}
	if n := w.stats.writeFailures.Load(); n != 1 {
		t.Fatalf("failures = %d, want 1", n)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.pending.empty() {
		t.Fatalf("a fenced writer keeps nothing pending: %+v", w.pending)
	}
}
