package dashboard

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

func occurrence(
	opened, lasted time.Duration, fix incident.Fix,
) incident.Occurrence {
	return incident.Occurrence{Opened: testEpoch.Add(opened),
		Resolved: testEpoch.Add(opened + lasted), Fix: fix}
}

func TestDetailSeenBeforeCountsTheLastThirtyDays(t *testing.T) {
	p := testIncident("a", "shop", -time.Hour)
	p.Previous = "old"
	p.History = []incident.Occurrence{
		occurrence(-40*24*time.Hour, time.Hour, incident.FixNone),
		occurrence(-3*24*time.Hour, 20*time.Minute, incident.FixRollback),
		occurrence(-5*time.Hour, 10*time.Minute, incident.FixConfig),
	}
	old := testIncident("old", "shop", -5*time.Hour)
	d := detailOf(t, newTestServer(p, old), "a")

	s := d.SeenBefore
	if s == nil || s.InWindow != 2 || s.WindowDays != 30 || s.Capped ||
		s.PreviousID != "old" || len(s.Recent) != 3 {
		t.Fatalf("seenBefore = %+v", s)
	}
	last := s.Recent[0]
	if !last.Opened.Equal(testEpoch.Add(-5*time.Hour)) ||
		last.Fix != "config fix" || last.Resolved.Sub(last.Opened) !=
		10*time.Minute {
		t.Errorf("last = %+v", last)
	}
}

func TestDetailSeenBeforeIsAbsentWithoutHistory(t *testing.T) {
	p := testIncident("a", "shop", 0)
	p.Previous = "gone"
	if d := detailOf(t, newTestServer(p), "a"); d.SeenBefore != nil {
		t.Errorf("seenBefore = %+v", d.SeenBefore)
	}
	p.History = []incident.Occurrence{occurrence(-time.Hour, time.Minute,
		"")}
	d := detailOf(t, newTestServer(p), "a")
	if d.SeenBefore == nil || d.SeenBefore.PreviousID != "" {
		t.Errorf("an unknown previous incident is linked: %+v",
			d.SeenBefore)
	}
}

func TestDetailPhasesShowFailingQuietAndRecovering(t *testing.T) {
	p := testIncident("a", "shop", -time.Hour)
	p.History = []incident.Occurrence{
		occurrence(-30*time.Hour, time.Hour, ""),
		occurrence(-3*time.Hour, time.Hour, ""),
	}
	p.RecoveringSince = testEpoch.Add(-10 * time.Minute)
	got := phasesOf(p, testEpoch)
	want := []Phase{
		{State: phaseFailing, From: testEpoch.Add(-3 * time.Hour),
			To:    testEpoch.Add(-2 * time.Hour),
			Began: testEpoch.Add(-30 * time.Hour)},
		{State: phaseQuiet, From: testEpoch.Add(-2 * time.Hour),
			To: testEpoch.Add(-time.Hour)},
		{State: phaseFailing, From: testEpoch.Add(-time.Hour),
			To: testEpoch.Add(-10 * time.Minute)},
		{State: phaseRecovering, From: testEpoch.Add(-10 * time.Minute)},
	}
	if len(got) != len(want) {
		t.Fatalf("phases = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("phase %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDetailPhasesClipALongIncidentToADay(t *testing.T) {
	p := testIncident("a", "shop", -72*time.Hour)
	p.State = incident.Resolved
	p.Resolved = testEpoch.Add(-time.Hour)
	got := phasesOf(p, testEpoch)
	if len(got) != 1 || got[0].State != phaseFailing ||
		!got[0].From.Equal(p.Resolved.Add(-phaseWindow)) ||
		!got[0].To.Equal(p.Resolved) {
		t.Errorf("phases = %+v", got)
	}
	if want := p.Opened; !got[0].Began.Equal(want) {
		t.Errorf("the first phase hides when it began: %+v", got[0])
	}
	if phasesOf(incident.Incident{}, testEpoch) != nil {
		t.Error("an incident without an open time has phases")
	}
}

// A window that cuts the middle of a long incident's story keeps its
// start on the first phase; an incident that fits has nothing to say.
func TestDetailPhasesKeepTheFirstStartWhenTheWindowCutsIt(t *testing.T) {
	p := testIncident("a", "shop", -2*time.Hour)
	p.History = []incident.Occurrence{
		occurrence(-100*time.Hour, time.Hour, ""),
		occurrence(-60*time.Hour, time.Hour, ""),
	}
	got := phasesOf(p, testEpoch)
	if len(got) == 0 ||
		!got[0].Began.Equal(testEpoch.Add(-100*time.Hour)) {
		t.Fatalf("phases = %+v", got)
	}
	if !got[0].From.After(got[0].Began) {
		t.Errorf("first phase starts at %v, not inside the window",
			got[0].From)
	}
	short := testIncident("b", "shop", -time.Hour)
	for _, ph := range phasesOf(short, testEpoch) {
		if !ph.Began.IsZero() {
			t.Errorf("an unclipped phase says it began: %+v", ph)
		}
	}
}

// Updates the incident dropped from the middle of its timeline show as
// a gap event right where they were, with the count.
func TestTimelineShowsWhereUpdatesWereNotKept(t *testing.T) {
	events := []incident.Event{
		{At: testEpoch, Text: "first"},
		{At: testEpoch.Add(time.Minute), Text: "second", Gap: 12},
		{At: testEpoch.Add(time.Hour), Text: "newest"},
	}
	got := toTimeline(events)
	if len(got) != 4 || got[2].Kind != kindGap || got[2].Gap != 12 ||
		got[2].Text != "12 earlier updates not kept" ||
		got[1].Text != "second" || got[3].Text != "newest" {
		t.Fatalf("timeline = %+v", got)
	}
}
