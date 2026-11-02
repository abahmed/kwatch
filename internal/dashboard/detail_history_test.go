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
		{phaseFailing, testEpoch.Add(-3 * time.Hour),
			testEpoch.Add(-2 * time.Hour)},
		{phaseQuiet, testEpoch.Add(-2 * time.Hour),
			testEpoch.Add(-time.Hour)},
		{phaseFailing, testEpoch.Add(-time.Hour),
			testEpoch.Add(-10 * time.Minute)},
		{phaseRecovering, testEpoch.Add(-10 * time.Minute), time.Time{}},
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
	if phasesOf(incident.Incident{}, testEpoch) != nil {
		t.Error("an incident without an open time has phases")
	}
}
