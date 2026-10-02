package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

func startRun(
	ctx context.Context, e *Engine,
) <-chan error {
	done := make(chan error, 1)
	go func() { done <- e.Run(ctx) }()
	return done
}

func waitFor(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for engine progress")
	}
}

func TestEngineRunStopsOnCancelAndSavesFinalState(t *testing.T) {
	st := &memStore{}
	clock := &fakeClock{
		now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC),
		ch:  make(chan time.Time),
	}
	progress := make(chan struct{}, 8)
	log := &sinkLog{}
	e := newTestEngine(t, clock, log.sink, func(d *Dependencies) {
		d.Store = st
		d.Progress = func() { progress <- struct{}{} }
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := startRun(ctx, e)

	e.Submit(ctx, inventory.Observation{Kind: inventory.Observed})
	waitFor(t, progress)
	e.SourcesSynced()
	waitFor(t, progress)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
	saved, prints := st.counts()
	if saved < 1 {
		t.Errorf("incident saves = %d, want the final save", saved)
	}
	if prints == 0 {
		t.Error("fingerprints were not saved after reconcile")
	}
}

func TestEngineRunReportsRestoreFailures(t *testing.T) {
	tests := map[string]*memStore{
		"incidents":    {loadErr: errStore},
		"fingerprints": {printsErr: errStore},
	}
	for name, st := range tests {
		t.Run(name, func(t *testing.T) {
			clock := &fakeClock{now: time.Now(), ch: make(chan time.Time)}
			e := newTestEngine(t, clock, (&sinkLog{}).sink,
				func(d *Dependencies) { d.Store = st })

			err := e.Run(context.Background())

			if !errors.Is(err, errStore) {
				t.Fatalf("Run = %v, want store error", err)
			}
		})
	}
}

func TestEngineRunWakesOnTimer(t *testing.T) {
	clock := &fakeClock{now: time.Now(), ch: make(chan time.Time, 1)}
	progress := make(chan struct{}, 8)
	e := newTestEngine(t, clock, (&sinkLog{}).sink,
		func(d *Dependencies) {
			d.Progress = func() { progress <- struct{}{} }
		})
	ctx, cancel := context.WithCancel(context.Background())
	done := startRun(ctx, e)

	clock.ch <- clock.now
	waitFor(t, progress)
	cancel()

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestEngineSubmitBlocksWhenFullUntilDrained(t *testing.T) {
	e := newTestEngine(t, &fakeClock{now: time.Now()},
		(&sinkLog{}).sink, nil)
	e.inbox.pending = make([]inventory.Observation, maxPending)
	returned := make(chan struct{})
	go func() {
		e.Submit(context.Background(), inventory.Observation{})
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("Submit returned while the queue was full")
	case <-time.After(20 * time.Millisecond):
	}
	e.drain()

	waitFor(t, returned)
	if len(e.inbox.pending) != 1 {
		t.Errorf("pending = %d, want 1", len(e.inbox.pending))
	}
}

func TestEngineSubmitReturnsWhenContextEndsWhileFull(t *testing.T) {
	e := newTestEngine(t, &fakeClock{now: time.Now()},
		(&sinkLog{}).sink, nil)
	e.inbox.pending = make([]inventory.Observation, maxPending)
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan struct{})
	go func() {
		e.Submit(ctx, inventory.Observation{})
		close(returned)
	}()

	cancel()

	waitFor(t, returned)
	if len(e.inbox.pending) != maxPending {
		t.Errorf("pending = %d, observation must not be queued", len(e.inbox.pending))
	}
}

func TestEngineSaveSurvivesStoreFailures(t *testing.T) {
	st := &memStore{saveErr: errStore, savePrintErr: errStore}
	e := newTestEngine(t, &fakeClock{now: time.Now()},
		(&sinkLog{}).sink, func(d *Dependencies) { d.Store = st })
	e.storage.reconciled = true

	e.save()
	e.storage.incidents.flush()

	saved, prints := st.counts()
	if saved != 1 || prints != 1 {
		t.Errorf("saves = %d/%d, want both attempted", saved, prints)
	}
}

func TestEngineSaveSkipsFingerprintsBeforeReconcile(t *testing.T) {
	st := &memStore{}
	e := newTestEngine(t, &fakeClock{now: time.Now()},
		(&sinkLog{}).sink, func(d *Dependencies) { d.Store = st })

	e.save()
	e.storage.incidents.flush()

	if _, prints := st.counts(); prints != 0 {
		t.Errorf("fingerprints saved before reconcile: %d", prints)
	}
}

func TestEngineTimerUsesEarliestDeadline(t *testing.T) {
	clock := &fakeClock{now: time.Now(), ch: make(chan time.Time, 1)}
	e := newTestEngine(t, clock, (&sinkLog{}).sink, nil)
	checks := newRechecks()

	got := e.timer(checks, clock.now.Add(time.Second))

	if got != clock.ch {
		t.Fatal("timer must come from the injected clock")
	}
}

func TestEngineInScopeDropsOutOfScopeDecisions(t *testing.T) {
	e := newTestEngine(t, &fakeClock{now: time.Now()},
		(&sinkLog{}).sink, func(d *Dependencies) {
			d.InScope = func(p incident.Incident) bool {
				return p.ID == "keep"
			}
		})
	in := []incident.Decision{
		{Incident: incident.Incident{ID: "keep"}},
		{Incident: incident.Incident{ID: "drop"}},
	}

	out := e.announcer.inScope(in)

	if len(out) != 1 || out[0].Incident.ID != "keep" {
		t.Fatalf("inScope = %v, want only keep", out)
	}
}
