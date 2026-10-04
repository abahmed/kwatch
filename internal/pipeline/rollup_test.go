package pipeline

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// rollupHarness is an engine whose sink keeps every message, delivered
// or carried.
func rollupHarness(
	t *testing.T, now time.Time,
) (*Engine, *[]notification.Message) {
	t.Helper()
	var seen []notification.Message
	sink := func(_ context.Context, _ incident.Decision, m notification.Message) {
		seen = append(seen, m)
	}
	return newTestEngine(t, &fakeClock{now: now}, sink, nil), &seen
}

// Two or more announcements made in one tick go as one roll-up. The
// page-tier one still reaches the paging tools on its own; the others
// are recorded as carried by the roll-up.
func TestEngineRollupSendsOneMessageForSimultaneousAnnouncements(
	t *testing.T,
) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	page := announce("node")
	page.Incident.Tier = incident.Page
	update := announce("other")
	update.Action = incident.Update

	rest, rolled := e.announcer.collectRollup(context.Background(), now,
		[]incident.Decision{page, announce("a"), update, announce("b")})

	if !rolled || len(rest) != 1 || rest[0].Action != incident.Update {
		t.Fatalf("only the update passes through, rest = %+v", rest)
	}
	if len(*seen) != 4 {
		t.Fatalf("want roll-up + paging + 2 carried, got %d: %+v",
			len(*seen), *seen)
	}
	rollup := (*seen)[0]
	if !rollup.IsSummary() || !strings.HasPrefix(rollup.Key, "rollup/") ||
		!strings.Contains(rollup.Note, "three new problems") {
		t.Fatalf("roll-up = %+v", rollup)
	}
	if !(*seen)[1].PagingOnly || (*seen)[1].Carrier != "" {
		t.Fatalf("the page goes to paging tools on its own: %+v", (*seen)[1])
	}
	for _, m := range (*seen)[2:] {
		if m.Carrier != "roll-up" {
			t.Fatalf("want carried by the roll-up, got %+v", m)
		}
	}
	rollups := e.announcer.startup.summary.Rollups
	if len(rollups) != 1 || rollups[0].Key != rollup.Key ||
		len(rollups[0].Incidents) != 3 {
		t.Fatalf("roll-up not remembered: %+v", rollups)
	}
}

// One announcement is sent on its own; nothing to roll up.
func TestEngineRollupLeavesASingleAnnouncementAlone(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)

	rest, rolled := e.announcer.collectRollup(context.Background(), now,
		[]incident.Decision{announce("a")})

	if rolled || len(rest) != 1 || len(*seen) != 0 {
		t.Fatalf("rolled=%v rest=%+v seen=%d", rolled, rest, len(*seen))
	}
}

// The first own message of an incident a roll-up listed is written as an
// announcement, and the roll-up closes once all it listed resolved.
func TestEngineRollupFollowsAndCloses(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	e, seen := rollupHarness(t, now)
	e.announcer.collectRollup(context.Background(), now,
		[]incident.Decision{announce("a"), announce("b")})
	update := announce("a")
	update.Action, update.Incident.Revision = incident.Update, 2

	first := e.announcer.followSummary(update)
	if first.Action != incident.Announce {
		t.Fatalf("first own message = %v, want an announcement", first.Action)
	}
	if again := e.announcer.followSummary(update); again.Action !=
		incident.Update {
		t.Fatalf("later messages stay updates, got %v", again.Action)
	}

	// a and b are unknown to the manager, so they count as closed.
	e.announcer.startup.checkSummary = true
	sent := e.announcer.closeSummary(context.Background())

	last := (*seen)[len(*seen)-1]
	if !sent || last.Status != notification.StatusResolved ||
		!strings.HasPrefix(last.Key, "rollup/") ||
		!strings.Contains(last.Note, "two problems") {
		t.Fatalf("sent=%v last=%+v", sent, last)
	}
	if len(e.announcer.startup.summary.Rollups) != 0 {
		t.Fatalf("closed roll-up still kept: %+v",
			e.announcer.startup.summary.Rollups)
	}
}

// A marker written before roll-ups existed, with the summary fields
// flat, still loads into the embedded listing.
func TestStartupStateReadsEarlierMarkers(t *testing.T) {
	old := `{"Complete":true,"Key":"startup/x","Incidents":["a"],` +
		`"Followed":["a"]}`
	var state StartupState
	if err := json.Unmarshal([]byte(old), &state); err != nil {
		t.Fatal(err)
	}
	if !state.Complete || state.Key != "startup/x" ||
		len(state.Incidents) != 1 || len(state.Followed) != 1 {
		t.Fatalf("state = %+v", state)
	}
	out, err := json.Marshal(StartupState{Complete: true,
		Listing: Listing{Key: "startup/x"},
		Rollups: []Listing{{Key: "rollup/y", Incidents: []string{"b"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"Key":"startup/x"`) ||
		!strings.Contains(string(out), `"Rollups":[{"Key":"rollup/y"`) {
		t.Fatalf("marker = %s", out)
	}
}
