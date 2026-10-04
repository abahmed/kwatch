package compose

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// "This is the second time this week" counts the times people heard
// about, not blips that recovered before anyone was told.
func TestRecurrenceSentenceCountsHeardOccurrencesOnly(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	heard := incident.Occurrence{Opened: now.Add(-24 * time.Hour), Heard: true}
	blip := incident.Occurrence{Opened: now.Add(-2 * time.Hour)}

	f := caseFacts{now: now, p: incident.Incident{
		Occurrences: []time.Time{now.Add(-2 * time.Hour), now},
		History:     []incident.Occurrence{blip},
	}}
	if got := recurrenceSentences(f); len(got) != 0 {
		t.Fatalf("a blip nobody heard of is not a previous time: %+v", got)
	}

	f.p.History = []incident.Occurrence{heard, blip}
	got := recurrenceSentences(f)
	if len(got) != 1 || got[0].text != "This is the second time this week." {
		t.Fatalf("want the heard occurrence counted once, got %+v", got)
	}
}
