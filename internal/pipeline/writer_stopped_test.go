package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The store may only be closed once every writer has returned. A history
// writer still running at its deadline must keep the store open.
func TestEngineWriterStoppedWaitsForHistoryWriter(t *testing.T) {
	done := make(chan struct{})
	e := &Engine{storage: &persistence{history: &historyRecorder{
		writer: &historyWriter{done: done},
	}}}

	if e.WriterStopped() {
		t.Fatal("history writer still running: WriterStopped must be false")
	}
	close(done)
	if !e.WriterStopped() {
		t.Fatal("all writers returned: WriterStopped must be true")
	}
}

// Run returns before it starts the incident writer when restore fails.
// A writer that never ran is stopped: the store may be closed.
func TestEngineWriterStoppedWhenRestoreFailed(t *testing.T) {
	clock := &fakeClock{now: time.Now(), ch: make(chan time.Time)}
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Store = &memStore{loadErr: errStore}
	})

	if err := e.Run(context.Background()); !errors.Is(err, errStore) {
		t.Fatalf("Run = %v, want the restore error", err)
	}
	if !e.WriterStopped() {
		t.Fatal("writer never started: WriterStopped must be true")
	}
}
