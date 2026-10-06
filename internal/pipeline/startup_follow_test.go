package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
)

// failingSince is an announcement whose only finding started at since.
func failingSince(id string, since time.Time) incident.Decision {
	d := announcement(id)
	finding := detection.Finding{Entity: inventory.CoreID("pod", "ns", id),
		Reason: "CrashLoop", Since: since}
	d.Incident.Opened = since.Add(time.Minute)
	d.Incident.Members = map[detection.Key]detection.Finding{
		finding.Key(): finding}
	return d
}

func TestEngineStartupHoldsOnlyWhatFailedBeforeSync(t *testing.T) {
	synced := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: synced}, (&sinkLog{}).sink, nil)
	e.announcer.collect.Startup.Until = synced.Add(announce.StartupWindow)
	old := failingSince("old", synced.Add(-time.Hour))
	fresh := failingSince("fresh", synced.Add(30*time.Second))

	rest, _ := e.announcer.collect.CollectStartup(context.Background(),
		synced.Add(time.Minute), []incident.Decision{old, fresh})

	if len(rest) != 1 || rest[0].Incident.ID != "fresh" {
		t.Fatalf("a problem that began after sync is news, rest = %+v",
			rest)
	}
	collected := e.announcer.collect.Startup.Collected
	if len(collected) != 1 || collected[0].Incident.ID != "old" {
		t.Fatalf("startup = %+v, want only the old problem held",
			collected)
	}
}

func TestEngineFirstMessageAfterSummaryIsAnnouncement(t *testing.T) {
	e := newTestEngine(t, &fakeClock{}, (&sinkLog{}).sink, nil)
	e.announcer.collect.Startup.Summary = announce.StartupState{Complete: true,
		Listing: announce.Listing{Key: "startup/x", Incidents: []string{"a"}}}
	update := incident.Decision{Action: incident.Update,
		Incident: incident.Incident{ID: "a", Revision: 2}}

	first := e.announcer.collect.Follow(update)
	second := e.announcer.collect.Follow(update)

	if first.Action != incident.Announce {
		t.Fatalf("first own message = %v, want an announcement",
			first.Action)
	}
	if second.Action != incident.Update {
		t.Fatalf("later messages stay updates, got %v", second.Action)
	}
	if update.Action != incident.Update {
		t.Fatal("the caller's decision must not change")
	}
}

func TestEngineSummaryResolvesOnlyWhenSomeoneWasNotTold(t *testing.T) {
	cases := map[string]struct {
		resolved []string
		want     bool
	}{
		"every incident said it resolved":    {[]string{"a", "b"}, false},
		"one closed without its own resolve": {[]string{"a"}, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			log := &sinkLog{}
			e := newTestEngine(t, &fakeClock{}, log.sink, nil)
			e.announcer.collect.Startup.Summary = announce.StartupState{Complete: true,
				Listing: announce.Listing{Key: "startup/x",
					Incidents: []string{"a", "b"}, Resolved: c.resolved}}
			e.announcer.collect.Startup.CheckSummary = true

			sent := e.announcer.collect.CloseListings(context.Background())

			if sent != c.want {
				t.Fatalf("summary resolve sent = %v, want %v", sent, c.want)
			}
			resolves := 0
			for _, d := range log.all() {
				if d.Reason == "startup summary resolved" {
					resolves++
				}
			}
			if (resolves == 1) != c.want {
				t.Fatalf("summary resolves = %d", resolves)
			}
			if len(e.announcer.collect.Startup.Summary.Incidents) != 0 {
				t.Fatalf("summary still waits: %+v", e.announcer.collect.Startup.Summary)
			}
		})
	}
}
