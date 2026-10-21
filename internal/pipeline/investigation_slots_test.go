package pipeline

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

func TestEngineHoldsAnnouncementsWithoutASlotUntilTheWaitEnds(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)
	var first, rest []incident.Decision
	for i := range investigationWorkers {
		first = append(first, announcement(string(rune('a'+i))))
	}
	for i := range investigationQueue + 2 {
		rest = append(rest, announcement(string(rune('A'+i))))
	}
	e.announcer.deliver(context.Background(), clock.now, first)
	for range investigationWorkers {
		<-g.started
	}

	e.announcer.deliver(context.Background(), clock.now, rest)

	if got := len(sink.all()); got != 0 {
		t.Fatalf("sent %d at once, want every announcement held", got)
	}
	want := investigationWorkers + investigationQueue + 2
	if len(e.announcer.held) != want {
		t.Fatalf("held %d, want %d", len(e.announcer.held), want)
	}
	if s := e.Stats(); s.InvestigationsSkipped != 2 {
		t.Fatalf("skipped = %d, want 2", s.InvestigationsSkipped)
	}
	// Nothing frees a slot: the two without one go out, without output,
	// when the wait ends.
	e.announcer.expireHeld(context.Background(), clock.now.Add(outputWait))
	if got := len(sink.all()); got != want {
		t.Fatalf("sent %d after the wait, want all %d", got, want)
	}
	close(g.release)
}

func TestEngineRetriesASkippedInvestigationWhileHeld(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	sink := &sinkLog{}
	e := investigatingEngine(t, clock, g, sink)
	startPool(t, e)
	ctx := context.Background()
	var first, rest []incident.Decision
	for i := range investigationWorkers {
		first = append(first, announcement(string(rune('a'+i))))
	}
	for i := range investigationQueue + 1 {
		rest = append(rest, announcement(string(rune('A'+i))))
	}
	lastID := string(rune('A' + investigationQueue))
	e.announcer.deliver(ctx, clock.now, first)
	for range investigationWorkers {
		<-g.started
	}
	e.announcer.deliver(ctx, clock.now, rest)
	total := len(first) + len(rest)

	close(g.release) // the running investigations end and free slots
	for len(sink.all()) < total {
		e.announcer.attachOutput(ctx, receive(t, e))
		e.announcer.retryUnsent(ctx, clock.now)
	}

	var last *incident.Decision
	for _, m := range sink.all() {
		if m.Incident.ID == lastID {
			last = &m
		}
	}
	if last == nil || len(last.Facts.Output) != 1 ||
		last.Facts.Output[0] != "output of "+lastID {
		t.Fatalf("the skipped announcement went without output: %+v", last)
	}
	if s := e.Stats(); s.InvestigationsSkipped != 1 {
		t.Fatalf("skipped = %d, want it counted once", s.InvestigationsSkipped)
	}
}

func TestEngineOpensInvestigationsLoudestTierFirst(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)}
	g := newGatedInvestigator()
	e := investigatingEngine(t, clock, g, &sinkLog{})
	// No workers: the queue alone decides how many investigations start.
	var opened []incident.Incident
	for i := range investigationQueue + 2 {
		tier := incident.Digest
		if i >= investigationQueue {
			tier = incident.Page // opened last
		}
		opened = append(opened, incident.Incident{
			ID: fmt.Sprintf("i%02d", i), Tier: tier})
	}

	e.announcer.investigateAll(opened, clock.now)

	for _, id := range []string{"i16", "i17"} {
		if state := e.announcer.evidence[id]; state == nil || !state.running {
			t.Errorf("page incident %s got no investigation slot", id)
		}
	}
	if started := len(e.announcer.evidence); started != investigationQueue {
		t.Errorf("%d investigations started, want %d",
			started, investigationQueue)
	}
}
