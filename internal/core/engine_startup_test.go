package core

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/problem"
)

func announce(id string) problem.Decision {
	return problem.Decision{
		Action: problem.Announce, Problem: problem.Problem{ID: id},
	}
}

func TestEngineCollectStartupHoldsAnnouncementsInWindow(t *testing.T) {
	log := &sinkLog{}
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, log.sink, nil)
	e.startupUntil = now.Add(time.Minute)
	resolve := problem.Decision{Action: problem.Resolve}

	rest := e.collectStartup(context.Background(), now,
		[]problem.Decision{announce("a"), resolve})

	if len(rest) != 1 || rest[0].Action != problem.Resolve {
		t.Fatalf("rest = %v, want only the resolve", rest)
	}
	if len(e.startup) != 1 || len(log.all()) != 0 {
		t.Fatal("announcement must be held, nothing sent yet")
	}
}

func TestEngineCollectStartupSendsSummaryAfterWindow(t *testing.T) {
	log := &sinkLog{}
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, log.sink, nil)
	e.startupUntil = now
	e.startup = []problem.Decision{announce("a")}

	rest := e.collectStartup(context.Background(), now.Add(time.Second),
		nil)

	if len(rest) != 0 {
		t.Fatalf("rest = %v", rest)
	}
	sent := log.all()
	if len(sent) != 1 || sent[0].Reason != "startup summary" {
		t.Fatalf("sent = %v, want one startup summary", sent)
	}
	if !e.startupUntil.IsZero() || e.startup != nil {
		t.Error("startup collection must reset")
	}
}

func TestEngineCollectStartupEmptyWindowSendsNothing(t *testing.T) {
	log := &sinkLog{}
	now := time.Now()
	e := newTestEngine(t, &fakeClock{now: now}, log.sink, nil)
	e.startupUntil = now

	e.collectStartup(context.Background(), now.Add(time.Hour), nil)

	if len(log.all()) != 0 || !e.startupUntil.IsZero() {
		t.Fatal("no summary expected for an empty startup")
	}
}

func TestEngineReconcileDowntimeStartsColdWindowOnce(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink, nil)
	e.coldStart = true

	e.reconcileDowntime()
	first := e.startupUntil
	e.reconcileDowntime()

	if want := now.Add(startupWindow); !first.Equal(want) {
		t.Fatalf("startupUntil = %v, want %v", first, want)
	}
	if !e.reconciled || !e.startupUntil.Equal(first) {
		t.Fatal("second reconcile must be a no-op")
	}
}

func TestEngineRestoreWarmStartKeepsProblemsQuiet(t *testing.T) {
	now := time.Now()
	st := &memStore{records: []problem.Record{{ID: "p1"}}}
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink,
		func(d *Dependencies) { d.Store = st })

	if err := e.restore(); err != nil {
		t.Fatal(err)
	}

	if e.coldStart {
		t.Error("restored problems mean a warm start")
	}
}

func TestEngineTickIdleDecidesNothing(t *testing.T) {
	log := &sinkLog{}
	var asked []string
	now := time.Now()
	e := newTestEngine(t, &fakeClock{now: now}, log.sink,
		func(d *Dependencies) {
			d.Investigate = func(
				_ context.Context, p problem.Problem,
			) []string {
				asked = append(asked, p.ID)
				return []string{"line"}
			}
		})

	next, decided := e.tick(context.Background(), now)

	if decided || !next.IsZero() || len(asked) != 0 {
		t.Fatalf("idle tick: next=%v decided=%v", next, decided)
	}
}
