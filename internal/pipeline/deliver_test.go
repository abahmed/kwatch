package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// gatedInvestigator blocks every investigation until release is closed or
// its context ends, and reports each start on started.
type gatedInvestigator struct {
	started chan string
	release chan struct{}
	ended   chan error
}

func newGatedInvestigator() *gatedInvestigator {
	return &gatedInvestigator{
		started: make(chan string, 64),
		release: make(chan struct{}),
		ended:   make(chan error, 64),
	}
}

func (g *gatedInvestigator) investigate(
	ctx context.Context, p incident.Incident,
) Result {
	g.started <- p.ID
	select {
	case <-g.release:
		g.ended <- nil
		return Result{Output: []string{"output of " + p.ID},
			Evidence: []incident.Fact{
				{Kind: incident.FactError, Text: "error of " + p.ID}}}
	case <-ctx.Done():
		g.ended <- ctx.Err()
		return Result{}
	}
}

// Plan implements Investigator.
func (g *gatedInvestigator) Plan(p incident.Incident) (Investigation, bool) {
	return funcInvestigator(g.investigate).Plan(p)
}

// funcInvestigator plans every incident as one investigation that runs
// the function.
type funcInvestigator func(context.Context, incident.Incident) Result

// Plan implements Investigator.
func (f funcInvestigator) Plan(p incident.Incident) (Investigation, bool) {
	return Investigation{Kind: "test", Run: func(ctx context.Context) Result {
		return f(ctx, p)
	}}, true
}

func investigatingEngine(
	t *testing.T, clock *fakeClock, g *gatedInvestigator, sink *sinkLog,
) *Engine {
	t.Helper()
	return newTestEngine(t, clock, sink.sink, func(d *Dependencies) {
		d.Investigator = g
	})
}

func startPool(t *testing.T, e *Engine) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	e.announcer.pool.start(ctx)
	t.Cleanup(func() {
		cancel()
		waitFor(t, e.announcer.pool.done)
	})
	return cancel
}

func receive(t *testing.T, e *Engine) investigationResult {
	t.Helper()
	select {
	case r := <-e.announcer.outputs():
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an investigation result")
	}
	return investigationResult{}
}

func TestEngineSlowInvestigationNeverDelaysOtherDecisions(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)
	update := incident.Decision{Action: incident.Update,
		Incident: incident.Incident{ID: "other"}}

	e.announcer.deliver(context.Background(), clock.now,
		[]incident.Decision{announce("a"), update})

	if got := sink.all(); len(got) != 1 || got[0].Incident.ID != "other" {
		t.Fatalf("delivered %+v, want the update at once", got)
	}
	if id := <-g.started; id != "a" {
		t.Fatalf("investigated %q, want a", id)
	}
	close(g.release)
}

func TestEngineAttachesOutputThatArrivesInTime(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	close(g.release)
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)

	e.announcer.deliver(context.Background(), clock.now,
		[]incident.Decision{announce("a")})
	e.announcer.attachOutput(context.Background(), receive(t, e))

	got := sink.all()
	if len(got) != 1 || len(got[0].Output) != 1 ||
		got[0].Output[0] != "output of a" {
		t.Fatalf("delivered %+v, want the announcement with output", got)
	}
	if len(e.announcer.held) != 0 || e.announcer.pool.inFlight != 0 {
		t.Fatalf("held=%d inFlight=%d after delivery",
			len(e.announcer.held), e.announcer.pool.inFlight)
	}
}

func TestEngineSendsHeldAnnouncementWithoutLateOutput(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)
	e.announcer.deliver(context.Background(), clock.now,
		[]incident.Decision{announce("a")})

	e.announcer.expireHeld(context.Background(), clock.now.Add(outputWait-1))
	if got := sink.all(); len(got) != 0 {
		t.Fatalf("sent %+v before the output wait ended", got)
	}
	if next := e.announcer.nextHeld(); !next.Equal(clock.now.Add(outputWait)) {
		t.Fatalf("nextHeld = %v, want the end of the wait", next)
	}
	e.announcer.expireHeld(context.Background(), clock.now.Add(outputWait))
	close(g.release)
	e.announcer.attachOutput(context.Background(), receive(t, e))

	got := sink.all()
	if len(got) != 1 || got[0].Output != nil {
		t.Fatalf("delivered %+v, want one announcement without output", got)
	}
	if s := e.Stats(); s.InvestigationsLate != 1 {
		t.Fatalf("late = %d, want 1", s.InvestigationsLate)
	}
}

func TestEngineUpdateReleasesHeldAnnouncementFirst(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)
	resolve := incident.Decision{Action: incident.Resolve,
		Incident: incident.Incident{ID: "a"}}

	e.announcer.deliver(context.Background(), clock.now,
		[]incident.Decision{announce("a")})
	e.announcer.deliver(context.Background(), clock.now,
		[]incident.Decision{resolve})

	got := sink.all()
	if len(got) != 2 || got[0].Action != incident.Announce ||
		got[1].Action != incident.Resolve {
		t.Fatalf("delivered %+v, want announce then resolve", got)
	}
	close(g.release)
}

func TestEngineSendsAtOnceWhenEveryInvestigationSlotIsBusy(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)
	var first, rest []incident.Decision
	for i := range investigationWorkers {
		first = append(first, announce(string(rune('a'+i))))
	}
	for i := range investigationQueue + 2 {
		rest = append(rest, announce(string(rune('A'+i))))
	}
	e.announcer.deliver(context.Background(), clock.now, first)
	for range investigationWorkers {
		<-g.started
	}

	e.announcer.deliver(context.Background(), clock.now, rest)

	if got := len(sink.all()); got != 2 {
		t.Fatalf("sent %d at once, want the 2 without a slot", got)
	}
	want := investigationWorkers + investigationQueue
	if len(e.announcer.held) != want {
		t.Fatalf("held %d, want %d", len(e.announcer.held), want)
	}
	if s := e.Stats(); s.InvestigationsSkipped != 2 {
		t.Fatalf("skipped = %d, want 2", s.InvestigationsSkipped)
	}
	close(g.release)
}

func TestEngineDeliversWithoutInvestigator(t *testing.T) {
	sink := &sinkLog{}
	e := newTestEngine(t, &fakeClock{}, sink.sink, nil)

	e.announcer.deliver(context.Background(), time.Time{},
		[]incident.Decision{announce("a")})

	if got := sink.all(); len(got) != 1 || got[0].Output != nil {
		t.Fatalf("delivered %+v", got)
	}
}

func TestEngineInvestigationPoolStopsOnCancel(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	e := investigatingEngine(t, clock, g, &sinkLog{})
	ctx, cancel := context.WithCancel(context.Background())
	e.announcer.pool.start(ctx)
	e.announcer.deliver(ctx, clock.now, []incident.Decision{announce("a")})
	<-g.started

	cancel()

	if err := <-g.ended; err == nil {
		t.Fatal("a running investigation must see the cancellation")
	}
	waitFor(t, e.announcer.pool.done)
}

func TestEngineInvestigationCarriesDeadline(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	deadlines := make(chan bool, 1)
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Investigator = funcInvestigator(func(
			ctx context.Context, _ incident.Incident,
		) Result {
			_, ok := ctx.Deadline()
			deadlines <- ok
			return Result{}
		})
	})
	startPool(t, e)

	e.announcer.deliver(context.Background(), clock.now,
		[]incident.Decision{announce("a")})

	if !<-deadlines {
		t.Fatal("investigation must carry a deadline")
	}
	if outputWait > investigationTimeout {
		t.Fatal("an investigation must be allowed the whole output wait")
	}
}

// A material change is investigated again when its evidence is old, and
// the update waits for the fresh result like an announcement does.
func TestEngineReinvestigatesMaterialChange(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	close(g.release)
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)
	e.announcer.deliver(context.Background(), clock.now,
		[]incident.Decision{announce("a")})
	e.announcer.attachOutput(context.Background(), receive(t, e))
	update := incident.Decision{Action: incident.Update,
		Reason:   incident.ReasonMaterialChange,
		Incident: incident.Incident{ID: "a", Revision: 2}}

	later := clock.now.Add(reinvestigateAfter + time.Second)
	e.announcer.deliver(context.Background(), later,
		[]incident.Decision{update})

	if len(sink.all()) != 1 {
		t.Fatalf("the update must wait for the new investigation: %+v",
			sink.all())
	}
	e.announcer.attachOutput(context.Background(), receive(t, e))
	got := sink.all()
	if len(got) != 2 || got[1].Action != incident.Update {
		t.Fatalf("delivered %+v, want the update after its evidence", got)
	}
	if e.announcer.evidence["a"].seq != 2 {
		t.Fatalf("seq = %d, want a second investigation",
			e.announcer.evidence["a"].seq)
	}
}

// A cause revision carries its cause already and is sent at once.
func TestEngineDoesNotHoldCauseRevisions(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)
	revised := incident.Decision{Action: incident.Update,
		Reason:   incident.ReasonCauseRevised,
		Incident: incident.Incident{ID: "a", Revision: 2}}

	e.announcer.deliver(context.Background(), clock.now,
		[]incident.Decision{revised})

	if len(sink.all()) != 1 {
		t.Fatalf("a cause revision must go at once: %+v", sink.all())
	}
	close(g.release)
}
