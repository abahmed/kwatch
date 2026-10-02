package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"
)

// budgetTimer fires only the shutdown budget, when the test says so,
// and counts how often the budget was asked for.
type budgetTimer struct {
	mu     sync.Mutex
	asked  int
	budget chan time.Time
}

func (b *budgetTimer) after(d time.Duration) <-chan time.Time {
	if d != shutdownBudget {
		return nil // batch waits never fire
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.asked++
	return b.budget
}

func (b *budgetTimer) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.asked
}

// Every worker waits within one shared deadline: a stuck final write
// costs the budget once, not once per worker in turn.
func TestEngineShutdownSharesOneDeadline(t *testing.T) {
	gate := make(chan struct{})
	t.Cleanup(func() { close(gate) })
	st := &memStore{entered: make(chan struct{}, 4), gate: gate}
	timer := &budgetTimer{budget: make(chan time.Time)}
	e := newTestEngine(t, &fakeClock{now: time.Now()}, (&sinkLog{}).sink,
		func(d *Dependencies) {
			d.Store = st
			d.Timer = timer.after
		})
	stop := e.startWorkers(context.Background())
	stopped := make(chan struct{})

	go func() {
		stop()
		close(stopped)
	}()
	waitFor(t, st.entered) // the final write is stuck in the store
	timer.budget <- time.Time{}

	waitFor(t, stopped)
	if got := timer.count(); got != 1 {
		t.Fatalf("shutdown asked for %d deadlines, want one shared", got)
	}
	if got := e.Stats().ShutdownTimeouts; got != 1 {
		t.Fatalf("shutdown timeouts = %d, want the stuck writer", got)
	}
}

func TestSharedDeadlineClosesForEveryWaiter(t *testing.T) {
	fire := make(chan time.Time, 1)
	deadline, release := sharedDeadline(
		func(time.Duration) <-chan time.Time { return fire }, time.Second)
	defer release()

	fire <- time.Time{}

	for range 3 {
		select {
		case <-deadline:
		case <-time.After(5 * time.Second):
			t.Fatal("a waiter missed the shared deadline")
		}
	}
	release() // releasing twice is safe
}
