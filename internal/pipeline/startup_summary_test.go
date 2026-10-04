package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// coldHarness starts a cold session with one crashing workload.
func coldHarness(t *testing.T, start time.Time) (*harness, func()) {
	t.Helper()
	h := newHarness(t, start)
	h.engine.announcer.startup.coldStart = true
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("queue")
	d.Status.ReadyReplicas = *d.Spec.Replicas
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	crashing := crashingPod("queue-a", rs.Name, "n1", start)
	h.add(kube.PodSchema{}, crashing)
	h.engine.reconcileDowntime()
	recover := func() {
		healthy := pod("queue-a", rs.Name, "n1", true, h.now)
		healthy.ResourceVersion = "next"
		h.engine.Submit(ctxBackground(), kube.NewTranslator(
			kube.PodSchema{}).Updated(crashing, healthy, h.now)...)
	}
	return h, recover
}

func heldRecords(h *harness) (held, total int) {
	for _, r := range h.engine.deps.Incidents.Export() {
		total++
		if r.Held {
			held++
		}
	}
	return held, total
}

func TestEngineStartupHeldAnnouncementIsNotPersistedAsAnnounced(
	t *testing.T,
) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h, _ := coldHarness(t, start)

	h.run(start.Add(90*time.Second), 10*time.Second)

	if len(h.decisions) != 0 {
		t.Fatalf("nothing is sent inside the window:\n%s", joinTitles(h))
	}
	held, total := heldRecords(h)
	if held != 1 || total != 1 {
		t.Fatalf("held = %d of %d, want the announcement held", held, total)
	}
	fresh := incident.NewManager(incident.Config{}, nil)
	fresh.Restore(h.engine.deps.Incidents.Export(), start.Add(time.Hour))
	if got := fresh.Incidents()[0]; got.State != incident.Settling {
		t.Fatalf("restart inside the window must re-announce, got %v",
			got.State)
	}
}

// resolvedBy reports whether a Resolve decision for incident id was
// delivered.
func resolvedBy(h *harness, id string) bool {
	for i, d := range h.decisions {
		if d.Action == incident.Resolve && h.messages[i].Key == id {
			return true
		}
	}
	return false
}

// TestEngineStartupSummaryReleasesAndLaterResolves drives the engine by
// events, not by elapsed time: it steps until the startup summary is
// complete, then until the incident's own resolve was delivered, each
// within a bound on the simulated clock. Nothing waits on the wall
// clock, and no step count is assumed.
func TestEngineStartupSummaryReleasesAndLaterResolves(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h, recover := coldHarness(t, start)

	completed := func() bool {
		return h.engine.announcer.startup.summary.Complete
	}
	if !h.runUntil(completed,
		start.Add(5*time.Minute), 10*time.Second) {
		t.Fatalf("startup summary never completed:\n%s", joinTitles(h))
	}

	if len(h.messages) != 1 ||
		!strings.HasPrefix(h.messages[0].Key, "startup/") {
		t.Fatalf("want one per-session summary:\n%s", joinTitles(h))
	}
	if held, _ := heldRecords(h); held != 0 {
		t.Fatal("summary delivery must release held announcements")
	}
	listed := h.engine.announcer.startup.summary.Incidents
	if len(listed) != 1 || !h.engine.announcer.startup.summary.Complete {
		t.Fatalf("summary state = %+v", h.engine.announcer.startup.summary)
	}

	recover()
	resolvedIncident := h.runUntil(func() bool {
		return resolvedBy(h, listed[0])
	}, h.now.Add(10*time.Minute), 10*time.Second)
	// One more step lets a summary resolve, if the engine wrongly sent
	// one, follow the incident's.
	h.run(h.now, 10*time.Second)

	closedSummary := false
	for _, m := range h.messages {
		closedSummary = closedSummary ||
			(m.Key == h.messages[0].Key &&
				m.Status == notification.StatusResolved)
	}
	// The incident said it resolved; a second "all resolved" from the
	// summary would be a repeated recovery.
	if !resolvedIncident || closedSummary {
		t.Fatalf("want one recovery, from the incident itself:\n%s",
			joinTitles(h))
	}
	if len(h.engine.announcer.startup.summary.Incidents) != 0 {
		t.Fatalf("summary still waits: %+v", h.engine.announcer.startup.summary)
	}
}

// A pre-existing page-tier incident is held for the chat summary like
// any other, and its announcement goes at once to the providers that
// track alerts by key, marked for them only.
func TestEngineStartupHoldsPageTierAndPagesSeparately(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	var sent []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		if m.Carrier == "" {
			sent = append(sent, m)
		}
	}
	e := newTestEngine(t, &fakeClock{now: now}, sink, nil)
	e.announcer.startup.until = now.Add(time.Minute)
	page := announce("node")
	page.Incident.Tier = incident.Page

	rest, _ := e.announcer.collectStartup(context.Background(), now,
		[]incident.Decision{page, announce("a")})

	if len(rest) != 0 {
		t.Fatalf("page tier must be held for the summary, rest = %+v", rest)
	}
	if len(e.announcer.startup.collected) != 2 {
		t.Fatalf("collected = %+v, want both", e.announcer.startup.collected)
	}
	if len(sent) != 1 || !sent[0].PagingOnly || sent[0].Key != "node" {
		t.Fatalf("want one paging-only announcement of the page, got %+v",
			sent)
	}
	// The second time the same announcement is held, nothing is sent.
	e.announcer.collectStartup(context.Background(), now,
		[]incident.Decision{page})
	if len(sent) != 1 {
		t.Fatalf("a held page must be paged once, got %d messages", len(sent))
	}
}

// The close of an incident whose failures another incident took over is
// for alert-tracking providers only; chat reads about those failures in
// the other incident's update.
func TestEngineSupersededResolveIsPagingOnly(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink, nil)
	resolve := incident.Decision{Action: incident.Resolve,
		Incident: incident.Incident{ID: "a", SupersededBy: "b"}}
	plain := incident.Decision{Action: incident.Resolve,
		Incident: incident.Incident{ID: "c"}}

	if !e.announcer.write(resolve, now).PagingOnly {
		t.Fatal("a superseded resolve must be paging-only")
	}
	if e.announcer.write(plain, now).PagingOnly {
		t.Fatal("an ordinary resolve goes everywhere")
	}
}

func TestEngineStartupFoldsUpdatesAndDropsResolvesOfHeld(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e := newTestEngine(t, &fakeClock{now: now}, (&sinkLog{}).sink, nil)
	e.announcer.startup.until = now.Add(time.Minute)
	update := incident.Decision{Action: incident.Update,
		Incident: incident.Incident{ID: "a", Revision: 2}}
	resolve := incident.Decision{Action: incident.Resolve,
		Incident: incident.Incident{ID: "b"}}

	rest, _ := e.announcer.collectStartup(context.Background(), now,
		[]incident.Decision{announce("a"), announce("b")})
	rest2, _ := e.announcer.collectStartup(context.Background(), now,
		[]incident.Decision{update, resolve})

	if len(rest)+len(rest2) != 0 {
		t.Fatalf("held incident decisions leaked: %v %v", rest, rest2)
	}
	collected := e.announcer.startup.collected
	if len(collected) != 1 || collected[0].Incident.Revision != 2 ||
		collected[0].Action != incident.Announce {
		t.Fatalf("startup = %+v", collected)
	}
}

func TestEngineRestoreUsesStartupMarker(t *testing.T) {
	records := []incident.Record{{ID: "p1"}}
	cases := map[string]struct {
		marker *StartupState
		want   bool
	}{
		"incomplete marker is cold":       {&StartupState{}, true},
		"complete marker is warm":         {&StartupState{Complete: true}, false},
		"records without marker are warm": {nil, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			st := &memStore{records: records, startup: c.marker}
			e := newTestEngine(t, &fakeClock{}, (&sinkLog{}).sink,
				func(d *Dependencies) { d.Store = st })

			if err := e.restore(); err != nil {
				t.Fatal(err)
			}
			if e.announcer.startup.coldStart != c.want {
				t.Fatalf("coldStart = %v, want %v", e.announcer.startup.coldStart, c.want)
			}
		})
	}
}

func TestEngineRestoreColdStartWritesIncompleteMarker(t *testing.T) {
	st := &memStore{}
	e := newTestEngine(t, &fakeClock{}, (&sinkLog{}).sink,
		func(d *Dependencies) { d.Store = st })

	if err := e.restore(); err != nil {
		t.Fatal(err)
	}
	if !e.announcer.startup.coldStart || len(st.startups) != 1 ||
		st.startups[0].Complete {
		t.Fatalf("cold start must persist an open marker: %+v",
			st.startups)
	}
}

func TestEngineRestoreFailsOnMarkerError(t *testing.T) {
	st := &memStore{startupErr: errStore}
	e := newTestEngine(t, &fakeClock{}, (&sinkLog{}).sink,
		func(d *Dependencies) { d.Store = st })

	if err := e.restore(); err == nil {
		t.Fatal("marker load failure must fail restore")
	}
}

func TestIncidentStoreStartupMarkerRoundTrip(t *testing.T) {
	ps := NewIncidentStore(openTempStore(t))

	if _, found, err := ps.LoadStartup(); err != nil || found {
		t.Fatalf("empty store: found=%v err=%v", found, err)
	}
	want := StartupState{Complete: true, Listing: Listing{Key: "startup/x",
		Incidents: []string{"a"}, Followed: []string{"a"},
		Resolved: []string{"a"}}}
	if err := ps.SaveStartup(want); err != nil {
		t.Fatal(err)
	}
	got, found, err := ps.LoadStartup()
	if err != nil || !found || got.Key != want.Key ||
		len(got.Incidents) != 1 || !got.Complete ||
		len(got.Followed) != 1 || len(got.Resolved) != 1 {
		t.Fatalf("round trip = %+v found=%v err=%v", got, found, err)
	}
}

// Decisions the startup summary holds reach the sink marked as carried,
// except a page-tier announcement, which goes out as paging-only.
func TestEngineStartupRecordsHeldDecisionsForTheAuditLog(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	var seen []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		seen = append(seen, m)
	}
	e := newTestEngine(t, &fakeClock{now: now}, sink, nil)
	e.announcer.startup.until = now.Add(time.Minute)
	page := announce("node")
	page.Incident.Tier = incident.Page
	resolve := announce("pod")
	resolve.Action = incident.Resolve

	e.announcer.collectStartup(context.Background(), now,
		[]incident.Decision{page, announce("pod"), resolve})

	if len(seen) != 3 {
		t.Fatalf("want three sink calls, got %d: %+v", len(seen), seen)
	}
	if !seen[0].PagingOnly || seen[0].Carrier != "" {
		t.Fatalf("the page goes out paging-only, got %+v", seen[0])
	}
	for _, m := range seen[1:] {
		if m.Carrier != "startup summary" || m.PagingOnly {
			t.Fatalf("held decisions are carried by the summary, got %+v", m)
		}
	}
}
