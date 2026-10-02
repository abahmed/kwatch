package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

func announce(id string) incident.Decision {
	return incident.Decision{
		Action: incident.Announce, Incident: incident.Incident{ID: id},
	}
}

func TestEngineCollectStartupHoldsAnnouncementsInWindow(t *testing.T) {
	log := &sinkLog{}
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, log.sink, nil)
	e.announcer.startup.until = now.Add(time.Minute)
	resolve := incident.Decision{Action: incident.Resolve}

	rest, _ := e.announcer.collectStartup(context.Background(), now,
		[]incident.Decision{announce("a"), resolve})

	if len(rest) != 1 || rest[0].Action != incident.Resolve {
		t.Fatalf("rest = %v, want only the resolve", rest)
	}
	if len(e.announcer.startup.collected) != 1 || len(log.all()) != 0 {
		t.Fatal("announcement must be held, nothing sent yet")
	}
}

func TestEngineCollectStartupSendsSummaryAfterWindow(t *testing.T) {
	log := &sinkLog{}
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, log.sink, nil)
	e.announcer.startup.until = now
	e.announcer.startup.collected = []incident.Decision{announce("a")}

	rest, _ := e.announcer.collectStartup(context.Background(),
		now.Add(time.Second), nil)

	if len(rest) != 0 {
		t.Fatalf("rest = %v", rest)
	}
	sent := log.all()
	if len(sent) != 1 || sent[0].Reason != "startup summary" {
		t.Fatalf("sent = %v, want one startup summary", sent)
	}
	startup := e.announcer.startup
	if !startup.until.IsZero() || startup.collected != nil {
		t.Error("startup collection must reset")
	}
}

func TestEngineCollectStartupEmptyWindowSendsNothing(t *testing.T) {
	log := &sinkLog{}
	now := time.Now()
	e := newTestEngine(t, &fakeClock{now: now}, log.sink, nil)
	e.announcer.startup.until = now

	e.announcer.collectStartup(context.Background(), now.Add(time.Hour), nil)

	if len(log.all()) != 0 || !e.announcer.startup.until.IsZero() {
		t.Fatal("no summary expected for an empty startup")
	}
}

func TestEngineReconcileDowntimeStartsColdWindowOnce(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink, nil)
	e.announcer.startup.coldStart = true

	e.reconcileDowntime()
	first := e.announcer.startup.until
	e.reconcileDowntime()

	if want := now.Add(startupWindow); !first.Equal(want) {
		t.Fatalf("startupUntil = %v, want %v", first, want)
	}
	if !e.storage.reconciled || !e.announcer.startup.until.Equal(first) {
		t.Fatal("second reconcile must be a no-op")
	}
}

func TestEngineRestoreWarmStartKeepsIncidentsQuiet(t *testing.T) {
	now := time.Now()
	st := &memStore{records: []incident.Record{{ID: "p1"}}}
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink,
		func(d *Dependencies) { d.Store = st })

	if err := e.restore(); err != nil {
		t.Fatal(err)
	}

	if e.announcer.startup.coldStart {
		t.Error("restored incidents mean a warm start")
	}
}

func TestEngineTickIdleDecidesNothing(t *testing.T) {
	log := &sinkLog{}
	var asked []string
	now := time.Now()
	e := newTestEngine(t, &fakeClock{now: now}, log.sink,
		func(d *Dependencies) {
			d.Investigator = funcInvestigator(func(
				_ context.Context, p incident.Incident,
			) Result {
				asked = append(asked, p.ID)
				return Result{Output: []string{"line"}}
			})
		})

	next, decided := e.tick(context.Background(), now)

	if decided || !next.IsZero() || len(asked) != 0 {
		t.Fatalf("idle tick: next=%v decided=%v", next, decided)
	}
}
