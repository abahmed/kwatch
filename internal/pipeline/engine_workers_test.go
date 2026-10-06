package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// lockedClock is a fake clock safe to move while Run reads it.
type lockedClock struct {
	mu  sync.Mutex
	now time.Time
	ch  chan time.Time
}

func (c *lockedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *lockedClock) After(time.Duration) <-chan time.Time { return c.ch }

func (c *lockedClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
	c.ch <- c.now
}

// batchNow fires storage batches at once and never fires a shutdown
// deadline, so a test decides when the writer is stuck.
func batchNow(d time.Duration) <-chan time.Time {
	if d != writeBatch {
		return nil
	}
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func TestEngineRunSlowStoreNeverDelaysDecisions(t *testing.T) {
	st := &memStore{gate: make(chan struct{}),
		entered: make(chan struct{}, 8)}
	clock := &lockedClock{
		now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC),
		ch:  make(chan time.Time),
	}
	progress := make(chan struct{}, 64)
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Store = st
		d.Timer = batchNow
		d.Progress = func() { progress <- struct{}{} }
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := startRun(ctx, e)
	e.Submit(ctx, inventory.Observation{Kind: inventory.Observed})
	waitFor(t, progress)

	clock.advance(saveInterval)
	waitFor(t, progress)
	waitFor(t, st.entered)
	for range 3 {
		clock.advance(saveInterval)
		waitFor(t, progress)
	}

	close(st.gate)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if saved, _ := st.counts(); saved < 2 {
		t.Fatalf("writes = %d, want the stuck write and a later batch",
			saved)
	}
}

func TestEngineRunGivesUpOnStuckStoreAtShutdown(t *testing.T) {
	st := &memStore{gate: make(chan struct{}),
		entered: make(chan struct{}, 8)}
	clock := &lockedClock{now: time.Now(), ch: make(chan time.Time)}
	progress := make(chan struct{}, 64)
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Store = st
		d.Progress = func() { progress <- struct{}{} }
		d.Timer = func(time.Duration) <-chan time.Time {
			ch := make(chan time.Time, 1)
			ch <- time.Time{}
			return ch
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := startRun(ctx, e)
	e.Submit(ctx, inventory.Observation{Kind: inventory.Observed})
	waitFor(t, progress)
	clock.advance(saveInterval)
	waitFor(t, st.entered)

	cancel()

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := e.Stats().ShutdownTimeouts; n != 1 {
		t.Fatalf("shutdown timeouts = %d, want the stuck writer", n)
	}
	if e.WriterStopped() {
		t.Fatal("an abandoned writer must not report as stopped")
	}
	close(st.gate)
	<-e.storage.incidents.done
	if !e.WriterStopped() {
		t.Fatal("a finished writer must report as stopped")
	}
}

func TestEngineRunStopsRunningInvestigations(t *testing.T) {
	g := newGatedInvestigator()
	clock := &lockedClock{now: time.Now(), ch: make(chan time.Time)}
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Investigator = g
	})
	ctx, cancel := context.WithCancel(context.Background())
	plan, _ := g.Plan(incident.Incident{ID: "a"})
	e.announcer.pool.submit(investigationJob{id: "a", plan: plan})
	done := startRun(ctx, e)
	<-g.started

	cancel()

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-g.ended; err == nil {
		t.Fatal("the investigation must be cancelled")
	}
	waitFor(t, e.announcer.pool.done)
}

// A crash is announced on time while its investigation hangs: the loop
// keeps stepping, and the message goes without output after the wait.
func TestEngineHungInvestigationStillAnnounces(t *testing.T) {
	g := newGatedInvestigator()
	defer close(g.release)
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.Investigator = g
	})
	startPool(t, h.engine)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("reports")
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{},
		crashingPod("reports-a", rs.Name, "n1", start),
		crashingPod("reports-b", rs.Name, "n1", start))

	h.run(start.Add(10*time.Minute), 10*time.Second)

	if len(h.decisions) != 1 || h.decisions[0].Facts.Output != nil {
		t.Fatalf("decisions = %+v, want one announcement without output",
			h.decisions)
	}
	if id := <-g.started; id != h.decisions[0].Incident.ID {
		t.Fatalf("investigated %q, want the announced incident", id)
	}
	if s := h.engine.Stats(); s.InvestigationsLate != 1 {
		t.Fatalf("late = %d, want 1", s.InvestigationsLate)
	}
}

// A held announcement is persisted as not announced, so a restart before
// delivery announces it again.
func TestEngineHeldAnnouncementIsPersistedAsHeld(t *testing.T) {
	g := newGatedInvestigator()
	defer close(g.release)
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.Investigator = g
	})
	startPool(t, h.engine)
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("reports")
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	h.add(kube.PodSchema{},
		crashingPod("reports-a", rs.Name, "n1", start))

	for len(h.engine.announcer.held) == 0 &&
		h.now.Before(start.Add(10*time.Minute)) {
		h.run(h.now, time.Second)
	}

	if len(h.engine.announcer.held) != 1 {
		t.Fatal("the announcement must be held for its output")
	}
	held := 0
	for _, r := range h.engine.deps.Incidents.Export() {
		if r.Held {
			held++
		}
	}
	if held != 1 {
		t.Fatalf("held records = %d, want the waiting announcement", held)
	}
}
