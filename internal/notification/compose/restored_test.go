package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestRestoredSummaryListsIncidentsStillFailing(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	msg := Writer{}.RestoredSummary([]incident.Decision{
		podDecision("quiet", incident.Notify),
		podDecision("loud", incident.Page),
	}, now)

	if want := "kwatch restarted and two problems from before are " +
		"still failing."; msg.Title != want {
		t.Errorf("Title = %q, want %q", msg.Title, want)
	}
	if !strings.HasPrefix(msg.Key, notification.SummaryKeyPrefix) {
		t.Errorf("Key = %s, want a summary key", msg.Key)
	}
	if msg.Marker != notification.MarkerPage {
		t.Errorf("marker = %q, want the loudest tier", msg.Marker)
	}
	if strings.Index(msg.Note, "loud") > strings.Index(msg.Note, "quiet") {
		t.Errorf("paging problem should come first: %q", msg.Note)
	}
	if !strings.Contains(msg.Note, "Each gets its own message") {
		t.Errorf("note should say where updates go: %q", msg.Note)
	}
}

func TestRestoredSummarySingularReadsNaturally(t *testing.T) {
	msg := Writer{}.RestoredSummary([]incident.Decision{
		podDecision("a", incident.Notify)}, time.Time{})
	if !strings.Contains(msg.Title, "one problem from before is still") {
		t.Errorf("Title = %q", msg.Title)
	}
}
