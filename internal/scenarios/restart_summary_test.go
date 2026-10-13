package scenarios

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/replay"
)

// A nightly restart over a loud problem and a quiet, long-standing one:
// the restart summary names the loud one and only counts the quiet one,
// which the digest lists, instead of both being repeated every morning.
func TestRestartSummaryCountsDigestTierProblems(t *testing.T) {
	store := newPersistingStore()
	_, failing := restartFirstSession()
	c := failing.cluster
	hpa := agedMissingTargetHPA(c)
	c.list(hpa)
	c.after(45 * time.Minute)
	first := c.log()
	firstRun := restartReplay(t, first, store, replay.Options{
		SyncAt: first.Start, Tail: 5 * time.Minute})

	restartAt := firstRun.End.Add(restartGap)
	again := newCluster(scenarioStart, "")
	again.now = restartAt
	again.list(remaining(c)...)
	again.after(30 * time.Minute)
	second := again.log()
	second.Start = restartAt.UTC()
	secondRun := restartReplay(t, second, store, replay.Options{
		SyncAt: restartAt, Tail: 20 * time.Minute})

	var summary string
	for _, m := range secondRun.Messages {
		if strings.Contains(m.Title, "restarted") {
			summary = m.Title + " " + m.Note
		}
	}
	if summary == "" {
		t.Fatalf("no restart summary: %s", describeDecisions(secondRun))
	}
	if !strings.Contains(summary, "invoicer") {
		t.Errorf("the loud problem is not named: %s", summary)
	}
	if strings.Contains(summary, "thirdparty") {
		t.Errorf("a digest-tier problem is named: %s", summary)
	}
	if !strings.Contains(summary, "lower-priority") {
		t.Errorf("the digest-tier problem is not counted: %s", summary)
	}
}
