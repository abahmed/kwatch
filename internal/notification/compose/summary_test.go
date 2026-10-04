package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

func podDecision(name string, tier incident.Tier) incident.Decision {
	id := inventory.CoreID("pod", "ns", name)
	return announce(incident.Incident{
		ID: "inc-" + name, Root: id, Tier: tier, State: incident.Open,
		Members: members(detection.Finding{Entity: id, Reason: "Test",
			Severity: detection.Critical, Summary: "Pod is crash looping"}),
	})
}

func TestStartupSummaryCountsAndKeys(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	msg := Writer{}.StartupSummary([]incident.Decision{
		podDecision("a", incident.Notify)}, now)

	if want := "startup/20240115T100000.000Z"; msg.Key != want {
		t.Errorf("Key = %s, want %s", msg.Key, want)
	}
	if msg.Title != "kwatch started and found one problem that began "+
		"before it was watching." {
		t.Errorf("Title = %q", msg.Title)
	}
	if msg.Status != notification.StatusWarning ||
		msg.Marker != notification.MarkerNotify {
		t.Errorf("Status = %v, marker %q", msg.Status, msg.Marker)
	}
	if !strings.Contains(msg.Note, "Pod a in ns is crash looping.") {
		t.Errorf("summary should describe the problem: %q", msg.Note)
	}
}

func TestStartupSummaryLoudestFirstAndMarker(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	msg := Writer{}.StartupSummary([]incident.Decision{
		podDecision("quiet", incident.Notify),
		podDecision("loud", incident.Page),
	}, now)

	if msg.Marker != notification.MarkerPage {
		t.Errorf("marker = %q, want the loudest tier", msg.Marker)
	}
	if strings.Index(msg.Note, "loud") > strings.Index(msg.Note, "quiet") {
		t.Errorf("paging problem should come first: %q", msg.Note)
	}
}

func TestStartupSummaryBoundsDescribedProblems(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	var decisions []incident.Decision
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		decisions = append(decisions, podDecision(name, incident.Notify))
	}

	msg := Writer{}.StartupSummary(decisions, now)

	if strings.Contains(msg.Note, "pod f ") {
		t.Errorf("summary should stop after %d problems: %q",
			maxSummaryNamed, msg.Note)
	}
	if !strings.Contains(msg.Note, "Two more are not described here.") {
		t.Errorf("summary should count the rest: %q", msg.Note)
	}
	if !strings.Contains(msg.Note, "its own message") {
		t.Errorf("summary should say updates come separately: %q",
			msg.Note)
	}
}

func TestStartupKeyIsUniquePerSession(t *testing.T) {
	first := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	second := first.Add(90 * time.Second)

	if StartupKey(first) == StartupKey(second) {
		t.Fatal("two sessions must not share a summary key")
	}
	if StartupKey(first) == "startup" {
		t.Fatal("the fixed legacy key must not be reused")
	}
}

func TestStartupResolvedClosesSummaryConversation(t *testing.T) {
	key := StartupKey(time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC))

	msg := Writer{}.StartupResolved(key, 3)

	if msg.Key != key || msg.Status != notification.StatusResolved ||
		msg.Revision != 2 {
		t.Fatalf("unexpected resolve message: %+v", msg)
	}
	want := "✅ All three problems found when kwatch started have resolved."
	if msg.Note != want {
		t.Fatalf("note = %q, want %q", msg.Note, want)
	}
}

func TestStartupResolvedSingularReadsNaturally(t *testing.T) {
	msg := Writer{Cluster: "prod"}.StartupResolved("startup/x", 1)

	want := "✅ The problem found when kwatch (prod) started has resolved."
	if msg.Note != want {
		t.Fatalf("note = %q, want %q", msg.Note, want)
	}
}

func TestRollupNamesProblemsLoudestFirst(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	msg := Writer{}.Rollup([]incident.Decision{
		podDecision("quiet", incident.Notify),
		podDecision("loud", incident.Page),
	}, now)

	if want := "rollup/20240115T100000.000Z"; msg.Key != want {
		t.Errorf("Key = %s, want %s", msg.Key, want)
	}
	if msg.Title != "kwatch found two new problems at the same time." {
		t.Errorf("Title = %q", msg.Title)
	}
	if msg.Marker != notification.MarkerPage || !msg.IsSummary() {
		t.Errorf("marker = %q summary=%v", msg.Marker, msg.IsSummary())
	}
	if strings.Index(msg.Note, "loud") > strings.Index(msg.Note, "quiet") {
		t.Errorf("paging problem should come first: %q", msg.Note)
	}
	if !strings.HasSuffix(msg.Note, eachOwnMessage) {
		t.Errorf("roll-up should say each problem gets its own message: %q",
			msg.Note)
	}
}

func TestRollupResolvedClosesTheConversation(t *testing.T) {
	msg := Writer{}.RollupResolved("rollup/x", 3)

	if msg.Key != "rollup/x" || msg.Revision != 2 ||
		msg.Status != notification.StatusResolved {
		t.Errorf("message = %+v", msg)
	}
	if msg.Title != "All three problems found at the same time have "+
		"resolved." {
		t.Errorf("Title = %q", msg.Title)
	}
}
