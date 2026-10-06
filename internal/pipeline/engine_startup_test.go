package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

func announcement(id string) incident.Decision {
	return incident.Decision{
		Action: incident.Announce, Incident: incident.Incident{ID: id},
	}
}

func TestEngineCollectStartupHoldsAnnouncementsInWindow(t *testing.T) {
	log := &sinkLog{}
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, log.sink, nil)
	e.announcer.collect.Startup.Until = now.Add(time.Minute)
	resolve := incident.Decision{Action: incident.Resolve}

	rest, _ := e.announcer.collect.CollectStartup(context.Background(), now,
		[]incident.Decision{announcement("a"), resolve})

	if len(rest) != 1 || rest[0].Action != incident.Resolve {
		t.Fatalf("rest = %v, want only the resolve", rest)
	}
	if len(e.announcer.collect.Startup.Collected) != 1 || len(log.all()) != 0 {
		t.Fatal("announcement must be held, nothing sent yet")
	}
}

func TestEngineCollectStartupSendsSummaryAfterWindow(t *testing.T) {
	log := &sinkLog{}
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, log.sink, nil)
	e.announcer.collect.Startup.Until = now
	e.announcer.collect.Startup.Collected = []incident.Decision{announcement("a")}

	rest, _ := e.announcer.collect.CollectStartup(context.Background(),
		now.Add(time.Second), nil)

	if len(rest) != 0 {
		t.Fatalf("rest = %v", rest)
	}
	sent := log.all()
	if len(sent) != 1 || sent[0].Reason != "startup summary" {
		t.Fatalf("sent = %v, want one startup summary", sent)
	}
	startup := e.announcer.collect.Startup
	if !startup.Until.IsZero() || startup.Collected != nil {
		t.Error("startup collection must reset")
	}
}

func TestEngineCollectStartupEmptyWindowSendsNothing(t *testing.T) {
	log := &sinkLog{}
	now := time.Now()
	e := newTestEngine(t, &fakeClock{now: now}, log.sink, nil)
	e.announcer.collect.Startup.Until = now

	e.announcer.collect.CollectStartup(
		context.Background(), now.Add(time.Hour), nil)

	if len(log.all()) != 0 || !e.announcer.collect.Startup.Until.IsZero() {
		t.Fatal("no summary expected for an empty startup")
	}
}

func TestEngineReconcileDowntimeStartsColdWindowOnce(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink, nil)
	e.announcer.collect.Startup.ColdStart = true

	e.reconcileDowntime()
	first := e.announcer.collect.Startup.Until
	e.reconcileDowntime()

	if want := now.Add(announce.StartupWindow); !first.Equal(want) {
		t.Fatalf("startupUntil = %v, want %v", first, want)
	}
	if !e.storage.reconciled || !e.announcer.collect.Startup.Until.Equal(first) {
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

	if e.announcer.collect.Startup.ColdStart {
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
			) testResult {
				asked = append(asked, p.ID)
				return testResult{Output: []string{"line"}}
			})
		})

	next, decided := e.tick(context.Background(), now)

	if decided || !next.IsZero() || len(asked) != 0 {
		t.Fatalf("idle tick: next=%v decided=%v", next, decided)
	}
}
