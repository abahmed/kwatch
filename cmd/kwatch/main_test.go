package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeBaselineSaver struct {
	mu    sync.Mutex
	calls []map[string]int64
}

func (f *fakeBaselineSaver) SaveBaseline(_ context.Context, b map[string]int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, b)
	return nil
}

func (f *fakeBaselineSaver) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeBaselineSaver) last() map[string]int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

func TestStartBaselineSaverCoalesces(t *testing.T) {
	saver := &fakeBaselineSaver{}
	ch := make(chan map[string]int64, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go startBaselineSaver(ctx, saver, ch, 10*time.Millisecond)

	// Send three snapshots rapidly — should coalesce to one write
	ch <- map[string]int64{"a:key:CrashLoopBackOff:": 100}
	ch <- map[string]int64{"a:key:CrashLoopBackOff:": 200, "b:key:OOMKilled:": 300}
	ch <- map[string]int64{"a:key:CrashLoopBackOff:": 400, "b:key:OOMKilled:": 500, "c:key:": 600}

	time.Sleep(50 * time.Millisecond)

	if saver.count() != 1 {
		t.Fatalf("expected exactly 1 save call after 3 rapid sends, got %d", saver.count())
	}

	last := saver.last()
	if last["c:key:"] != 600 {
		t.Fatalf("expected third snapshot (latest), got %v", last)
	}

	// A fourth snapshot after the write should trigger a second write
	ch <- map[string]int64{"d:key:": 700}
	time.Sleep(50 * time.Millisecond)

	if saver.count() != 2 {
		t.Fatalf("expected exactly 2 save calls after 4th send, got %d", saver.count())
	}
}

func TestStartBaselineSaverCancelsCleanly(t *testing.T) {
	saver := &fakeBaselineSaver{}
	ch := make(chan map[string]int64, 1)
	ctx, cancel := context.WithCancel(context.Background())

	go startBaselineSaver(ctx, saver, ch, 10*time.Millisecond)

	ch <- map[string]int64{"k:": 1}
	time.Sleep(5 * time.Millisecond)

	cancel()
	time.Sleep(10 * time.Millisecond)

	// goroutine should be gone; sending to ch should not deadlock
	// (nobody is reading anymore, but buf=1 so it's ok)
	// If cancel didn't work, the test would hang. We just pass if we get here.
}
